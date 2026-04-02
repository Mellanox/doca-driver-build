/*
 Copyright 2026, NVIDIA CORPORATION & AFFILIATES

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package entrypoint

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ModuleInfo holds parsed information about a kernel module from /proc/modules.
type ModuleInfo struct {
	// Name is the module name.
	Name string
	// UserCount is the reference count (number of users) from /proc/modules.
	UserCount int
	// DependsOn lists the modules that use (depend on) this one,
	// as parsed from field 4 of /proc/modules.
	DependsOn []string
}

// ParseProcModules reads and parses /proc/modules (or a compatible file) into a map keyed by module name.
// /proc/modules format: name size refcount dep1,dep2,... state address
// deps are comma-separated users of this module with a trailing comma, or "-" if none.
func ParseProcModules(procModulesPath string) (map[string]ModuleInfo, error) {
	data, err := os.ReadFile(procModulesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", procModulesPath, err)
	}

	modules := make(map[string]ModuleInfo)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue // malformed line, skip
		}

		name := fields[0]
		refCount, err := strconv.Atoi(fields[2])
		if err != nil {
			refCount = 0
		}

		var deps []string
		depsField := fields[3]
		if depsField != "-" {
			// deps are comma-separated with a trailing comma
			parts := strings.Split(strings.TrimRight(depsField, ","), ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					deps = append(deps, p)
				}
			}
		}

		modules[name] = ModuleInfo{
			Name:      name,
			UserCount: refCount,
			DependsOn: deps,
		}
	}

	return modules, nil
}

// ValidateUnloadSafety checks that each target module can be safely unloaded.
// For each target, it reads /sys/module/<mod>/holders/ to get the actual kernel module
// holders (modules that depend on this one), then compares the holder count against the
// refcount from /proc/modules. If refcount > holder count, there are unknown userspace
// processes holding the module and unloading would be unsafe.
// sysModulePath should be "/sys/module" in production or a temp dir for testing.
func ValidateUnloadSafety(targets []string, modules map[string]ModuleInfo, sysModulePath string) error {
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		info, found := modules[target]
		if !found {
			// Module not loaded — safe to proceed (nothing to unload)
			continue
		}

		// Read /sys/module/<target>/holders/ for actual kernel module users
		holdersDir := filepath.Join(sysModulePath, target, "holders")
		var holderNames []string
		holderCount := 0
		entries, err := os.ReadDir(holdersDir)
		if err == nil {
			for _, entry := range entries {
				holderNames = append(holderNames, entry.Name())
				holderCount++
			}
		}
		// If the directory doesn't exist, holderCount stays 0

		if info.UserCount > holderCount {
			userspaceCount := info.UserCount - holderCount
			errMsg := fmt.Sprintf(
				"module %q has refcount %d but only %d kernel module holder(s)",
				target, info.UserCount, holderCount)
			if holderCount > 0 {
				errMsg += fmt.Sprintf(" (kernel dependents: %v)", holderNames)
			}
			errMsg += fmt.Sprintf(
				"; %d unknown userspace process(es) are using this module — unsafe to unload."+
					" Hint: check 'lsof /dev/infiniband/*' or 'fuser /dev/infiniband/*'",
				userspaceCount)
			return fmt.Errorf("%s", errMsg)
		}
	}
	return nil
}

// ResolveUnloadOrder returns a topological ordering (leaf-first) of the target modules
// suitable for safe unloading. It repeatedly emits modules whose in-set user count is zero.
func ResolveUnloadOrder(targets []string, modules map[string]ModuleInfo) ([]string, error) {
	// Build the set of targets
	targetSet := make(map[string]bool)
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t != "" {
			targetSet[t] = true
		}
	}

	// Compute in-set dependency counts: for each target, how many other targets depend on it
	inSetUsers := make(map[string]int)
	for t := range targetSet {
		inSetUsers[t] = 0
	}

	// For each module in the target set, DependsOn lists modules that USE it (upward).
	// If dep is in t's DependsOn, dep depends on t, meaning t must be unloaded AFTER dep.
	// So t gets inSetUsers++ for each in-set module that uses it.
	for t := range targetSet {
		info, found := modules[t]
		if !found {
			continue
		}
		for _, dep := range info.DependsOn {
			if targetSet[dep] {
				inSetUsers[t]++
			}
		}
	}

	var order []string
	remaining := make(map[string]bool)
	for t := range targetSet {
		remaining[t] = true
	}

	for len(remaining) > 0 {
		var ready []string
		for t := range remaining {
			if inSetUsers[t] == 0 {
				ready = append(ready, t)
			}
		}

		if len(ready) == 0 {
			var stuck []string
			for t := range remaining {
				stuck = append(stuck, t)
			}
			return nil, fmt.Errorf("circular dependency detected among modules: %v", stuck)
		}

		// Sort ready for deterministic output
		sortStrings(ready)
		for _, t := range ready {
			order = append(order, t)
			delete(remaining, t)

			// When t is removed, decrement inSetUsers for modules whose DependsOn includes t.
			// DependsOn lists upward users, so if m.DependsOn contains t, then t uses m,
			// and removing t reduces m's in-set user count.
			for m := range remaining {
				info, found := modules[m]
				if !found {
					continue
				}
				for _, dep := range info.DependsOn {
					if dep == t {
						inSetUsers[m]--
					}
				}
			}
		}
	}

	return order, nil
}

// sortStrings sorts a slice of strings in place (simple insertion sort to avoid importing sort).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
