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

package driver

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

// New creates a new instance of the driver manager
func New(containerMode string, cfg config.Config,
	c cmd.Interface, h host.Interface, osWrapper wrappers.OSWrapper,
) Interface {
	return &driverMgr{
		cfg:           cfg,
		containerMode: containerMode,
		cmd:           c,
		host:          h,
		os:            osWrapper,
	}
}

// Interface is the interface exposed by the driver package.
type Interface interface {
	// PreStart validates environment variables and performs required initialization for
	// the requested containerMode
	PreStart(ctx context.Context) error
	// Build installs required dependencies and build the driver
	Build(ctx context.Context) error
	// Load the new driver version. Returns a boolean indicating whether the driver was loaded successfully.
	// The function will return false if the system already has the same driver version loaded.
	Load(ctx context.Context) (bool, error)
	// Unload the driver and replace it with the inbox driver. Returns a boolean indicating whether the driver was unloaded successfully.
	// The function will return false if the system already runs with inbox driver.
	Unload(ctx context.Context) (bool, error)
	// Clear cleanups the system by removing unended leftovers.
	Clear(ctx context.Context) error
}

type driverMgr struct {
	cfg           config.Config
	containerMode string

	cmd  cmd.Interface
	host host.Interface
	os   wrappers.OSWrapper
}

// PreStart is the default implementation of the driver.Interface.
func (d *driverMgr) PreStart(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	switch d.containerMode {
	case constants.DriverContainerModeSources:
		log.Info("Executing driver sources container")
		if d.cfg.NvidiaNicDriverPath == "" {
			err := fmt.Errorf("NVIDIA_NIC_DRIVER_PATH environment variable must be set")
			log.Error(err, "missing required environment variable")
			return err
		}
		log.V(1).Info("Drivers source", "path", d.cfg.NvidiaNicDriverPath)
		if err := d.prepareGCC(ctx); err != nil {
			return err
		}
		if d.cfg.NvidiaNicDriversInventoryPath != "" {
			info, err := os.Stat(d.cfg.NvidiaNicDriversInventoryPath)
			if err != nil {
				log.Error(err, "path from NVIDIA_NIC_DRIVERS_INVENTORY_PATH environment variable is not accessible",
					"path", d.cfg.NvidiaNicDriversInventoryPath)
				return err
			}
			if !info.IsDir() {
				log.Error(err, "path from NVIDIA_NIC_DRIVERS_INVENTORY_PATH is not a dir",
					"path", d.cfg.NvidiaNicDriversInventoryPath)
				return fmt.Errorf("NVIDIA_NIC_DRIVERS_INVENTORY_PATH is not a dir")
			}
			log.V(1).Info("use driver inventory", "path", d.cfg.NvidiaNicDriversInventoryPath)
		}
		log.V(1).Info("driver inventory path is not set, container will always recompile driver on startup")
		return nil
	case constants.DriverContainerModePrecompiled:
		log.Info("Executing precompiled driver container")
		return nil
	default:
		return fmt.Errorf("unknown containerMode")
	}
}

// Build is the default implementation of the driver.Interface.
func (d *driverMgr) Build(ctx context.Context) error {
	// TODO: Implement
	return nil
}

// Load is the default implementation of the driver.Interface.
func (d *driverMgr) Load(ctx context.Context) (bool, error) {
	// TODO: Implement
	return true, nil
}

// Unload is the default implementation of the driver.Interface.
func (d *driverMgr) Unload(ctx context.Context) (bool, error) {
	// TODO: Implement
	return true, nil
}

// Clear is the default implementation of the driver.Interface.
func (d *driverMgr) Clear(ctx context.Context) error {
	// TODO: Implement
	return nil
}

func (d *driverMgr) prepareGCC(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	// Get OS type first to check if it's OpenShift
	osType, err := d.host.GetOSType(ctx)
	if err != nil {
		return fmt.Errorf("failed to get OS type: %w", err)
	}

	// Check if OpenShift is detected - skip GCC setup for RHCOS/OpenShift
	if osType == constants.OSTypeOpenShift {
		log.V(1).Info("RHCOS detected (OpenShift), skipping GCC setup")
		return nil
	}

	// Extract GCC version from /proc/version
	gccVersion, majorVersion, err := d.extractGCCInfo(ctx)
	if err != nil {
		return err
	}
	if gccVersion == "" {
		log.V(1).Info("Could not extract GCC version from /proc/version")
		return nil
	}

	log.V(1).Info("Kernel compiled with GCC version", "version", gccVersion, "major", majorVersion)

	// Install and configure GCC based on OS type
	gccBinary, kernelGCCVer, err := d.installGCCForOS(ctx, osType, majorVersion)
	if err != nil {
		return err
	}

	// Set up alternatives for GCC binary
	return d.setupGCCAlternatives(ctx, gccBinary, kernelGCCVer)
}

// extractGCCInfo extracts GCC version information from /proc/version
func (d *driverMgr) extractGCCInfo(ctx context.Context) (string, int, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Read /proc/version to extract GCC version
	procVersion, err := d.os.ReadFile("/proc/version")
	if err != nil {
		return "", 0, fmt.Errorf("failed to read /proc/version: %w", err)
	}

	log.V(1).Info("Kernel version info", "proc_version", string(procVersion))

	// Extract GCC version using regex
	gccVersion, err := d.extractGCCVersion(string(procVersion))
	if err != nil {
		log.V(1).Info("Could not extract GCC version from /proc/version", "error", err)
		return "", 0, nil // Not a fatal error, continue without GCC setup
	}

	// Extract major version
	majorVersion, err := d.extractMajorVersion(gccVersion)
	if err != nil {
		return "", 0, fmt.Errorf("failed to extract major version from %s: %w", gccVersion, err)
	}

	return gccVersion, majorVersion, nil
}

// installGCCForOS installs GCC package based on OS type
func (d *driverMgr) installGCCForOS(ctx context.Context, osType string, majorVersion int) (string, string, error) {
	switch osType {
	case constants.OSTypeUbuntu:
		return d.installGCCUbuntu(ctx, majorVersion)
	case constants.OSTypeSLES:
		return d.installGCCSLES(ctx, majorVersion)
	case constants.OSTypeRedHat:
		return d.installGCCRedHat(ctx, majorVersion)
	default:
		return "", "", fmt.Errorf("unsupported OS type: %s", osType)
	}
}

// installGCCUbuntu installs GCC for Ubuntu
func (d *driverMgr) installGCCUbuntu(ctx context.Context, majorVersion int) (string, string, error) {
	log := logr.FromContextOrDiscard(ctx)
	kernelGCCVer := fmt.Sprintf("gcc-%d", majorVersion)

	log.V(1).Info("Installing GCC for Ubuntu", "package", kernelGCCVer)
	_, _, err := d.cmd.RunCommand(ctx, "apt-get", "-yq", "update")
	if err != nil {
		return "", "", fmt.Errorf("failed to update apt packages: %w", err)
	}
	_, _, err = d.cmd.RunCommand(ctx, "apt-get", "-yq", "install", kernelGCCVer)
	if err != nil {
		return "", "", fmt.Errorf("failed to install %s: %w", kernelGCCVer, err)
	}

	gccBinary := fmt.Sprintf("/usr/bin/%s", kernelGCCVer)
	return gccBinary, kernelGCCVer, nil
}

// installGCCSLES installs GCC for SLES
func (d *driverMgr) installGCCSLES(ctx context.Context, majorVersion int) (string, string, error) {
	log := logr.FromContextOrDiscard(ctx)
	kernelGCCVerPackage := fmt.Sprintf("gcc%d", majorVersion)
	kernelGCCVerBin := fmt.Sprintf("gcc-%d", majorVersion)

	log.V(1).Info("Installing GCC for SLES", "package", kernelGCCVerPackage)
	_, _, err := d.cmd.RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", kernelGCCVerPackage)
	if err != nil {
		return "", "", fmt.Errorf("failed to install %s: %w", kernelGCCVerPackage, err)
	}

	gccBinary := fmt.Sprintf("/usr/bin/%s", kernelGCCVerBin)
	return gccBinary, kernelGCCVerBin, nil
}

// installGCCRedHat installs GCC for RedHat
func (d *driverMgr) installGCCRedHat(ctx context.Context, majorVersion int) (string, string, error) {
	log := logr.FromContextOrDiscard(ctx)
	toolsetPackage := fmt.Sprintf("gcc-toolset-%d", majorVersion)

	log.V(1).Info("Checking for gcc-toolset availability", "package", toolsetPackage)

	// Check if gcc-toolset is available
	_, _, err := d.cmd.RunCommand(ctx, "dnf", "list", "available", toolsetPackage)
	if err == nil {
		// gcc-toolset version is available
		kernelGCCVer := fmt.Sprintf("gcc-toolset-%d-gcc", majorVersion)
		log.V(1).Info("Installing gcc-toolset for RedHat", "package", toolsetPackage)
		_, _, err = d.cmd.RunCommand(ctx, "dnf", "-q", "-y", "install", toolsetPackage)
		if err != nil {
			return "", "", fmt.Errorf("failed to install %s: %w", toolsetPackage, err)
		}
		gccBinary := fmt.Sprintf("/opt/rh/gcc-toolset-%d/root/usr/bin/gcc", majorVersion)
		return gccBinary, kernelGCCVer, nil
	}

	// Fall back to default gcc package
	log.V(1).Info("gcc-toolset not available, using default gcc package")
	kernelGCCVer := "gcc"
	_, _, err = d.cmd.RunCommand(ctx, "dnf", "-q", "-y", "install", "gcc")
	if err != nil {
		return "", "", fmt.Errorf("failed to install gcc: %w", err)
	}
	gccBinary := "/usr/bin/gcc"
	return gccBinary, kernelGCCVer, nil
}

// setupGCCAlternatives sets up GCC alternatives
func (d *driverMgr) setupGCCAlternatives(ctx context.Context, gccBinary, kernelGCCVer string) error {
	log := logr.FromContextOrDiscard(ctx)
	altGCCPrio := 200

	log.V(1).Info("Setting up GCC alternatives", "gcc_binary", gccBinary, "priority", altGCCPrio)
	_, _, err := d.cmd.RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", gccBinary, strconv.Itoa(altGCCPrio))
	if err != nil {
		return fmt.Errorf("failed to set up GCC alternatives: %w", err)
	}

	log.Info("Set GCC for driver compilation, matching kernel compiled version", "version", kernelGCCVer)
	return nil
}

// extractGCCVersion extracts GCC version from /proc/version string
func (d *driverMgr) extractGCCVersion(procVersion string) (string, error) {
	// Regex to match gcc version pattern: gcc followed by optional non-digit characters and then version number
	re := regexp.MustCompile(`(?i)gcc[^0-9]*([0-9]+\.[0-9]+\.[0-9]+)`)
	matches := re.FindStringSubmatch(procVersion)

	if len(matches) < 2 {
		return "", fmt.Errorf("no GCC version found in /proc/version")
	}

	return matches[1], nil
}

// extractMajorVersion extracts the major version number from a version string like "11.5.0"
func (d *driverMgr) extractMajorVersion(version string) (int, error) {
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid version format: %s", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse major version from %s: %w", version, err)
	}

	return major, nil
}
