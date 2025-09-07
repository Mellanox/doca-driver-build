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
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
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

// RedhatVersionInfo contains version information for RedHat-based distributions
type RedhatVersionInfo struct {
	// MajorVersion is the major version number (e.g., 8, 9)
	MajorVersion int
	// FullVersion is the complete version string (e.g., "8.4", "9.2")
	FullVersion string
	// OpenShiftVersion is the OpenShift version if running on RHCOS (e.g., "4.9")
	// If empty, this is not RHCOS
	OpenShiftVersion string
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
	// GetRedHatVersionInfo parses RedHat version information from /host/etc/os-release
	// and returns version details. Should only be called for RedHat-based distributions.
	GetRedHatVersionInfo(ctx context.Context) (*RedhatVersionInfo, error)
}

type host struct {
	cmd cmd.Interface
	os  wrappers.OSWrapper

	// Cache for OS type
	osTypeCache struct {
		value string
		err   error
		once  sync.Once
	}

	// Cache for RedHat version info
	redhatVersionCache struct {
		value *RedhatVersionInfo
		err   error
		once  sync.Once
	}
}

// GetOSType is the default implementation of the host.Interface.
func (h *host) GetOSType(ctx context.Context) (string, error) {
	h.osTypeCache.once.Do(func() {
		// Read /etc/os-release file to determine OS type
		osReleaseContent, err := h.os.ReadFile("/etc/os-release")
		if err != nil {
			h.osTypeCache.err = err
			return
		}

		osReleaseStr := string(osReleaseContent)
		osReleaseStr = strings.ToLower(osReleaseStr)

		// Check for Ubuntu (case insensitive)
		if strings.Contains(osReleaseStr, "ubuntu") {
			h.osTypeCache.value = constants.OSTypeUbuntu
			return
		}

		// Check for SLES (case insensitive)
		if strings.Contains(osReleaseStr, "sles") {
			h.osTypeCache.value = constants.OSTypeSLES
			return
		}

		// Default to redhat for other distributions (RHEL, CentOS, Fedora, etc.)
		h.osTypeCache.value = constants.OSTypeRedHat

		// Build the redhatVersionCache for RedHat-based systems
		h.buildRedHatVersionCache()

		// Check if this is actually an OpenShift system (has OpenShift version)
		if h.redhatVersionCache.value != nil && h.redhatVersionCache.value.OpenShiftVersion != "" {
			h.osTypeCache.value = constants.OSTypeOpenShift
		}
	})

	return h.osTypeCache.value, h.osTypeCache.err
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
	// UsedBy contains names of the modules that depends on this module.
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

// buildRedHatVersionCache builds the RedHat version cache by parsing /host/etc/os-release
func (h *host) buildRedHatVersionCache() {
	// Read /host/etc/os-release file
	osReleaseContent, err := h.os.ReadFile("/host/etc/os-release")
	if err != nil {
		h.redhatVersionCache.err = fmt.Errorf("failed to read /host/etc/os-release: %w", err)
		return
	}

	osReleaseStr := string(osReleaseContent)

	// Parse the os-release content
	versionInfo := &RedhatVersionInfo{}

	// Extract ID, VERSION_ID, RHEL_VERSION, and OPENSHIFT_VERSION
	var id, versionID, rhelVersion, openshiftVersion string

	idMatch := regexp.MustCompile(`(?m)^ID=(.+)$`).FindStringSubmatch(osReleaseStr)
	if len(idMatch) > 1 {
		id = strings.Trim(idMatch[1], `"`)
	}

	versionIDMatch := regexp.MustCompile(`(?m)^VERSION_ID=(.+)$`).FindStringSubmatch(osReleaseStr)
	if len(versionIDMatch) > 1 {
		versionID = strings.Trim(versionIDMatch[1], `"`)
	}

	rhelVersionMatch := regexp.MustCompile(`(?m)^RHEL_VERSION=(.+)$`).FindStringSubmatch(osReleaseStr)
	if len(rhelVersionMatch) > 1 {
		rhelVersion = strings.Trim(rhelVersionMatch[1], `"`)
	}

	openshiftVersionMatch := regexp.MustCompile(`(?m)^OPENSHIFT_VERSION=(.+)$`).FindStringSubmatch(osReleaseStr)
	if len(openshiftVersionMatch) > 1 {
		openshiftVersion = strings.Trim(openshiftVersionMatch[1], `"`)
	}

	if id == "rhcos" {
		// This is RHCOS - use OpenShift version logic
		if openshiftVersion != "" {
			versionInfo.OpenShiftVersion = openshiftVersion
		} else {
			versionInfo.OpenShiftVersion = versionID
		}
		if versionInfo.OpenShiftVersion == "" {
			versionInfo.OpenShiftVersion = constants.DefaultOpenShiftVersion
		}
		versionInfo.FullVersion = versionInfo.OpenShiftVersion
	} else {
		// For RHEL and other RedHat-based distros (CentOS, Fedora, etc.)
		versionInfo.FullVersion = rhelVersion
		if versionInfo.FullVersion == "" {
			versionInfo.FullVersion = versionID
		}
		if versionInfo.FullVersion == "" {
			versionInfo.FullVersion = constants.DefaultRHELVersion
		}

		// If OPENSHIFT_VERSION is present, this might be RHCOS with ID=rhel
		if openshiftVersion != "" {
			versionInfo.OpenShiftVersion = openshiftVersion
		}
	}

	// Extract major version from full version
	majorVersionStr := strings.Split(versionInfo.FullVersion, ".")[0]
	majorVersion, err := strconv.Atoi(majorVersionStr)
	if err != nil {
		h.redhatVersionCache.err = fmt.Errorf("failed to parse major version from '%s': %w", versionInfo.FullVersion, err)
		return
	}
	versionInfo.MajorVersion = majorVersion

	h.redhatVersionCache.value = versionInfo
}

// GetRedHatVersionInfo is the default implementation of the host.Interface.
func (h *host) GetRedHatVersionInfo(ctx context.Context) (*RedhatVersionInfo, error) {
	// First check if this is a RedHat-based system
	osType, err := h.GetOSType(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get OS type: %w", err)
	}

	if osType != constants.OSTypeRedHat && osType != constants.OSTypeOpenShift {
		return nil, fmt.Errorf("GetRedHatVersionInfo should only be called for RedHat-based distributions, got: %s", osType)
	}

	// Return the cached RedHat version info (built during GetOSType)
	return h.redhatVersionCache.value, h.redhatVersionCache.err
}
