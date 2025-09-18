/*
 Copyright 2025, NVIDIA CORPORATION & AFFILIATES

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

package host

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

// New initialize default implementation of the host.Interface.
func New(c cmd.Interface, osWrapper wrappers.OSWrapper) Interface {
	return &host{
		cmd: c,
		os:  osWrapper,
	}
}

// Interface is the interface exposed by the host package.
type Interface interface {
	// GetOSType returns the name of the operating system as a string.
	GetOSType(ctx context.Context) (string, error)
	// GetDebugInfo returns a string containing debug information about the OS,
	// such as kernel version and memory info. This information is printed to the debug log.
	GetDebugInfo(ctx context.Context) (string, error)
	// LsMod returns list of the loaded kernel modules.
	LsMod(ctx context.Context) (map[string]LoadedModule, error)
	// RmMod unload the kernel module.
	RmMod(ctx context.Context, module string) error
}

type host struct {
	cmd cmd.Interface
	os  wrappers.OSWrapper
}

// GetOSType is the default implementation of the host.Interface.
func (h *host) GetOSType(ctx context.Context) (string, error) {
	// TODO: add implementation
	return "", nil
}

// GetDebugInfo is the default implementation of the host.Interface.
func (h *host) GetDebugInfo(ctx context.Context) (string, error) {
	var debugInfo strings.Builder

	// Get OS release information
	osReleaseContent, err := h.os.ReadFile("/etc/os-release")
	if err != nil {
		debugInfo.WriteString(fmt.Sprintf("[os-release]: Error reading /etc/os-release: %v\n", err))
	} else {
		debugInfo.WriteString(fmt.Sprintf("[os-release]: %s\n", string(osReleaseContent)))
	}

	// Get kernel information
	stdout, stderr, err := h.cmd.RunCommand(ctx, "uname", "-a")
	if err != nil {
		debugInfo.WriteString(fmt.Sprintf("[uname -a]: Error executing uname -a: %v (stderr: %s)\n", err, stderr))
	} else {
		debugInfo.WriteString(fmt.Sprintf("[uname -a]: %s\n", stdout))
	}

	// Get memory information
	stdout, stderr, err = h.cmd.RunCommand(ctx, "free", "-m")
	if err != nil {
		debugInfo.WriteString(fmt.Sprintf("[free -m]: Error executing free -m: %v (stderr: %s)\n", err, stderr))
	} else {
		debugInfo.WriteString(fmt.Sprintf("[free -m]: %s\n", stdout))
	}

	return debugInfo.String(), nil
}

// LoadedModule contains information about loaded kernel module.
type LoadedModule struct {
	// Name of the kernel module.
	Name string
	// RefCount amount of refs to the module.
	RefCount int
	// UseBy contains names of the modules that depends on this module.
	UsedBy []string
}

// LsMod is the default implementation of the host.Interface.
func (h *host) LsMod(ctx context.Context) (map[string]LoadedModule, error) {
	// Execute lsmod command
	stdout, stderr, err := h.cmd.RunCommand(ctx, "lsmod")
	if err != nil {
		return nil, fmt.Errorf("failed to execute lsmod command: %w, stderr: %s", err, stderr)
	}

	// Parse the output
	modules := make(map[string]LoadedModule)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")

	// Skip the header line
	for i, line := range lines {
		if i == 0 {
			continue // Skip header line "Module                  Size  Used by"
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse each line: module_name size ref_count [dependent_modules]
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue // Skip malformed lines
		}

		moduleName := fields[0]
		refCountStr := fields[2]

		// Parse reference count
		refCount, err := strconv.Atoi(refCountStr)
		if err != nil {
			// If we can't parse the ref count, set it to 0
			refCount = 0
		}

		// Parse dependent modules (everything after the ref count)
		var usedBy []string
		if len(fields) > 3 {
			// Join all fields after ref count and split by comma
			dependentStr := strings.Join(fields[3:], " ")
			if dependentStr != "-" {
				// Split by comma and clean up each module name
				dependentModules := strings.Split(dependentStr, ",")
				for _, dep := range dependentModules {
					dep = strings.TrimSpace(dep)
					if dep != "" {
						usedBy = append(usedBy, dep)
					}
				}
			}
		}

		modules[moduleName] = LoadedModule{
			Name:     moduleName,
			RefCount: refCount,
			UsedBy:   usedBy,
		}
	}

	return modules, nil
}

// RmMod is the default implementation of the host.Interface.
func (h *host) RmMod(ctx context.Context, module string) error {
	// Execute rmmod command to unload the kernel module
	_, stderr, err := h.cmd.RunCommand(ctx, "rmmod", module)
	if err != nil {
		return fmt.Errorf("failed to unload kernel module %s: %w, stderr: %s", module, err, stderr)
	}
	return nil
}
