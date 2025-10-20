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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

const (
	kernelTypeStandard = "standard"
	kernelTypeRT       = "rt"
	kernelType64k      = "64k"
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
	cfg             config.Config
	containerMode   string
	newDriverLoaded bool

	cmd  cmd.Interface
	host host.Interface
	os   wrappers.OSWrapper
}

// PreStart is the default implementation of the driver.Interface.
func (d *driverMgr) PreStart(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	// Update CA certificates at the very beginning
	if err := d.updateCACertificates(ctx); err != nil {
		log.V(1).Info("Failed to update CA certificates", "error", err)
		// Non-fatal error, continue
	}

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
		} else {
			log.V(1).Info("driver inventory path is not set, container will always recompile driver on startup")
			return nil
		}
	case constants.DriverContainerModePrecompiled:
		log.Info("Executing precompiled driver container")
		return nil
	default:
		return fmt.Errorf("unknown containerMode")
	}
	return nil
}

// Build is the default implementation of the driver.Interface.
func (d *driverMgr) Build(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	// Only build for sources container mode
	if d.containerMode != constants.DriverContainerModeSources {
		log.V(1).Info("Skipping build for non-sources container mode", "mode", d.containerMode)
		return nil
	}

	// Get kernel version
	kernelVersion, err := d.host.GetKernelVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get kernel version: %w", err)
	}

	// Get OS type
	osType, err := d.host.GetOSType(ctx)
	if err != nil {
		return fmt.Errorf("failed to get OS type: %w", err)
	}

	// Check driver inventory and validate checksums
	shouldBuild, inventoryPath, err := d.checkDriverInventory(ctx, kernelVersion)
	if err != nil {
		return fmt.Errorf("failed to check driver inventory: %w", err)
	}

	if !shouldBuild {
		log.Info("Skipping driver build, reusing previously built packages", "kernel", kernelVersion)
	} else {
		// Create inventory directory
		if err := d.createInventoryDirectory(ctx, inventoryPath); err != nil {
			return fmt.Errorf("failed to create inventory directory: %w", err)
		}

		// Install OS-specific prerequisites
		log.V(1).Info("About to install prerequisites", "os", osType, "kernel", kernelVersion)
		if err := d.installPrerequisitesForOS(ctx, osType, kernelVersion); err != nil {
			return fmt.Errorf("failed to install prerequisites: %w", err)
		}

		// Build driver from source
		if err := d.buildDriverFromSource(ctx, d.cfg.NvidiaNicDriverPath, kernelVersion, osType); err != nil {
			return fmt.Errorf("failed to build driver from source: %w", err)
		}

		// Copy build artifacts to inventory
		if err := d.copyBuildArtifacts(ctx, d.cfg.NvidiaNicDriverPath, inventoryPath, osType); err != nil {
			return fmt.Errorf("failed to copy build artifacts: %w", err)
		}

		// Calculate and store checksum
		if d.cfg.NvidiaNicDriversInventoryPath != "" {
			if err := d.storeBuildChecksum(ctx, inventoryPath, kernelVersion); err != nil {
				return fmt.Errorf("failed to store build checksum: %w", err)
			}
		}

		// Fix source link if needed
		if err := d.fixSourceLink(ctx, kernelVersion); err != nil {
			log.V(1).Info("Failed to fix source link", "error", err)
			// Non-fatal error, continue
		}

		log.Info("Driver build completed successfully", "kernel", kernelVersion, "inventory", inventoryPath)
	}

	// Install the driver packages (always install, whether from cache or fresh build)
	if err := d.installDriver(ctx, inventoryPath, kernelVersion, osType); err != nil {
		return fmt.Errorf("failed to install driver: %w", err)
	}

	// Sync Ubuntu network configuration tools if running on Ubuntu
	if osType == constants.OSTypeUbuntu {
		if err := d.ubuntuSyncNetworkConfigurationTools(ctx); err != nil {
			return fmt.Errorf("failed to sync Ubuntu network configuration tools: %w", err)
		}
	}

	return nil
}

// Load is the default implementation of the driver.Interface.
func (d *driverMgr) Load(ctx context.Context) (bool, error) {
	if err := d.generateOfedModulesBlacklist(ctx); err != nil {
		return false, err
	}
	defer func() {
		if err := d.removeOfedModulesBlacklist(ctx); err != nil {
			log := logr.FromContextOrDiscard(ctx)
			log.Error(err, "Failed to remove OFED modules blacklist during cleanup")
		}
	}()

	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Loading driver modules")

	// Define modules to check
	modulesToCheck := []string{"mlx5_core", "mlx5_ib", "ib_core"}

	// Add NFS RDMA modules if enabled
	if d.cfg.EnableNfsRdma {
		modulesToCheck = append(modulesToCheck, "nvme_rdma", "rpcrdma")
	}

	// Check if loaded kernel modules match expected versions
	modulesMatch, err := d.checkLoadedKmodSrcverVsModinfo(ctx, modulesToCheck)
	if err != nil {
		return false, fmt.Errorf("failed to check module versions: %w", err)
	}

	if !modulesMatch {
		log.V(1).Info("Module versions don't match, restarting driver")

		// Restart driver
		if err := d.restartDriver(ctx); err != nil {
			return false, fmt.Errorf("failed to restart driver: %w", err)
		}

		// Mark that a new driver was loaded
		d.newDriverLoaded = true

		// Load NFS RDMA modules if enabled
		if d.cfg.EnableNfsRdma {
			if err := d.loadNfsRdma(ctx); err != nil {
				log.V(1).Info("Failed to load NFS RDMA modules", "error", err)
				// Non-fatal error, continue
			}
		}
	} else {
		log.V(1).Info("Loaded and candidate drivers are identical, skipping reload")
	}

	// Print loaded driver version
	if err := d.printLoadedDriverVersion(ctx); err != nil {
		log.V(1).Info("Failed to print driver version", "error", err)
		// Non-fatal error, continue
	}

	log.Info("Driver loaded successfully")
	return true, nil
}

// Unload is the default implementation of the driver.Interface.
func (d *driverMgr) Unload(ctx context.Context) (bool, error) {
	log := logr.FromContextOrDiscard(ctx)

	if d.newDriverLoaded {
		// Check if mlnxofedctl exists
		if _, err := d.os.Stat("/usr/sbin/mlnxofedctl"); err == nil {
			log.Info("Restoring Mellanox OFED Driver from host...")

			// Execute mlnxofedctl --alt-mods force-restart
			_, _, err := d.cmd.RunCommand(ctx, "/usr/sbin/mlnxofedctl", "--alt-mods", "force-restart")
			if err != nil {
				return false, fmt.Errorf("failed to restore driver with mlnxofedctl: %w", err)
			}

			// Print loaded driver version
			if err := d.printLoadedDriverVersion(ctx); err != nil {
				log.V(1).Info("Failed to print driver version after restore", "error", err)
				// Non-fatal error, continue
			}

			log.Info("Driver restored successfully")
			return true, nil
		} else {
			log.V(1).Info("mlnxofedctl not found, cannot restore driver")
		}
	} else {
		log.Info("Keeping currently loaded Mellanox OFED Driver...")
	}

	return false, nil
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

// generateOfedModulesBlacklist creates a blacklist file for OFED modules to prevent
// inbox or host OFED driver loading. This function writes module blacklist entries
// to the configured blacklist file.
func (d *driverMgr) generateOfedModulesBlacklist(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Generating OFED modules blacklist")

	// Create the blacklist file
	file, err := d.os.Create(d.cfg.OfedBlacklistModulesFile)
	if err != nil {
		log.Error(err, "Failed to create blacklist file", "file", d.cfg.OfedBlacklistModulesFile)
		return fmt.Errorf("failed to create blacklist file %s: %w", d.cfg.OfedBlacklistModulesFile, err)
	}
	defer file.Close()

	// Build the entire content first
	var content strings.Builder
	content.WriteString("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading\n\n")

	// Add blacklist entries for each module
	for _, module := range d.cfg.OfedBlacklistModules {
		module = strings.TrimSpace(module)
		if module == "" {
			continue
		}
		content.WriteString(fmt.Sprintf("blacklist %s\n", module))
		log.V(2).Info("Added module to blacklist", "module", module)
	}

	// Write all content at once
	if _, err := file.WriteString(content.String()); err != nil {
		log.Error(err, "Failed to write blacklist content to file")
		return fmt.Errorf("failed to write blacklist content to file: %w", err)
	}

	log.Info("Successfully generated OFED modules blacklist", "file", d.cfg.OfedBlacklistModulesFile, "modules", d.cfg.OfedBlacklistModules)
	return nil
}

// removeOfedModulesBlacklist removes the OFED modules blacklist file from the host.
// This function is typically called during cleanup or when the blacklist is no longer needed.
func (d *driverMgr) removeOfedModulesBlacklist(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("Removing OFED modules blacklist file")

	// Check if file exists before attempting to remove
	if _, err := d.os.Stat(d.cfg.OfedBlacklistModulesFile); os.IsNotExist(err) {
		log.V(1).Info("Blacklist file does not exist, nothing to remove", "file", d.cfg.OfedBlacklistModulesFile)
		return nil
	}

	// Remove the blacklist file
	if err := d.os.RemoveAll(d.cfg.OfedBlacklistModulesFile); err != nil {
		log.Error(err, "Failed to remove blacklist file", "file", d.cfg.OfedBlacklistModulesFile)
		return fmt.Errorf("failed to remove blacklist file %s: %w", d.cfg.OfedBlacklistModulesFile, err)
	}

	log.Info("Successfully removed OFED modules blacklist file", "file", d.cfg.OfedBlacklistModulesFile)
	return nil
}

// checkDriverInventory checks if driver inventory exists and validates checksums
func (d *driverMgr) checkDriverInventory(ctx context.Context, kernelVersion string) (bool, string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// If no inventory path is set, always build
	if d.cfg.NvidiaNicDriversInventoryPath == "" {
		inventoryPath := fmt.Sprintf("/tmp/nvidia_nic_driver_%s", time.Now().Format("02-01-2006_15-04-05"))
		return true, inventoryPath, nil
	}

	// Check if inventory directory exists
	inventoryPath := filepath.Join(d.cfg.NvidiaNicDriversInventoryPath, kernelVersion, d.cfg.NvidiaNicDriverVer)
	checksumPath := filepath.Join(d.cfg.NvidiaNicDriversInventoryPath, kernelVersion, d.cfg.NvidiaNicDriverVer+".checksum")

	// Check if inventory directory exists
	if _, err := d.os.Stat(inventoryPath); os.IsNotExist(err) {
		log.V(1).Info("Driver inventory directory does not exist, will build", "path", inventoryPath)
		return true, inventoryPath, nil
	} else if err != nil {
		return false, "", fmt.Errorf("failed to check inventory directory: %w", err)
	}

	// Check if checksum file exists
	if _, err := d.os.Stat(checksumPath); os.IsNotExist(err) {
		log.V(1).Info("No checksum file found, will rebuild", "path", checksumPath)
		return true, inventoryPath, nil
	} else if err != nil {
		return false, "", fmt.Errorf("failed to check checksum file: %w", err)
	}

	// Read stored checksum
	storedChecksum, err := d.os.ReadFile(checksumPath)
	if err != nil {
		log.V(1).Info("Failed to read stored checksum, will rebuild", "error", err)
		return true, inventoryPath, nil
	}

	// Calculate current checksum
	currentChecksum, err := d.calculateDriverInventoryChecksum(ctx, inventoryPath)
	if err != nil {
		log.V(1).Info("Failed to calculate current checksum, will rebuild", "error", err)
		return true, inventoryPath, nil
	}

	// Compare checksums
	if strings.TrimSpace(string(storedChecksum)) == currentChecksum {
		log.V(1).Info("Checksums match, skipping build", "checksum", currentChecksum)
		return false, inventoryPath, nil
	}

	log.V(1).Info("Checksums do not match, will rebuild", "stored", strings.TrimSpace(string(storedChecksum)), "current", currentChecksum)
	return true, inventoryPath, nil
}

// createInventoryDirectory creates the inventory directory
func (d *driverMgr) createInventoryDirectory(ctx context.Context, inventoryPath string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Creating inventory directory", "path", inventoryPath)
	_, _, err := d.cmd.RunCommand(ctx, "mkdir", "-p", inventoryPath)
	if err != nil {
		return fmt.Errorf("failed to create inventory directory %s: %w", inventoryPath, err)
	}

	return nil
}

// installPrerequisitesForOS installs OS-specific prerequisites
func (d *driverMgr) installPrerequisitesForOS(ctx context.Context, osType, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Installing prerequisites", "os", osType, "kernel", kernelVersion)

	switch osType {
	case constants.OSTypeUbuntu:
		return d.installUbuntuPrerequisites(ctx, kernelVersion)
	case constants.OSTypeSLES:
		return d.installSLESPrerequisites(ctx, kernelVersion)
	case constants.OSTypeRedHat, constants.OSTypeOpenShift:
		return d.installRedHatPrerequisites(ctx, kernelVersion)
	default:
		return fmt.Errorf("unsupported OS type: %s", osType)
	}
}

// installUbuntuPrerequisites installs Ubuntu-specific prerequisites
func (d *driverMgr) installUbuntuPrerequisites(ctx context.Context, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Installing Ubuntu prerequisites", "kernel", kernelVersion)

	// Check if this is an RT (realtime) kernel
	if strings.Contains(kernelVersion, "realtime") {
		log.V(1).Info("RT kernel identified, copying APT configuration from host")

		// Copy APT configuration from host for RT kernels
		_, _, err := d.cmd.RunCommand(ctx, "cp", "-r", "/host/etc/apt/*", "/etc/apt/")
		if err != nil {
			return fmt.Errorf("failed to copy APT configuration from host: %w", err)
		}
	}

	// Update package list
	_, _, err := d.cmd.RunCommand(ctx, "apt-get", "update")
	if err != nil {
		return fmt.Errorf("failed to update apt packages: %w", err)
	}

	// Install pkg-config and kernel headers
	_, _, err = d.cmd.RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-"+kernelVersion)
	if err != nil {
		return fmt.Errorf("failed to install Ubuntu prerequisites: %w", err)
	}

	return nil
}

// installSLESPrerequisites installs SLES-specific prerequisites
func (d *driverMgr) installSLESPrerequisites(ctx context.Context, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Installing SLES prerequisites", "kernel", kernelVersion)

	// Clean kernel version for SLES
	cleanedKernelVer := strings.TrimSuffix(kernelVersion, "-default")

	// Install kernel development package
	_, _, err := d.cmd.RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "kernel-default-devel="+cleanedKernelVer)
	if err != nil {
		return fmt.Errorf("failed to install SLES prerequisites: %w", err)
	}

	return nil
}

// installRedHatPrerequisites installs RedHat-specific prerequisites
func (d *driverMgr) installRedHatPrerequisites(ctx context.Context, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Installing RedHat prerequisites", "kernel", kernelVersion)

	// Get RedHat version information
	versionInfo, err := d.host.GetRedHatVersionInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get RedHat version info: %w", err)
	}

	// Enable OpenShift repositories if running on OpenShift
	if versionInfo.OpenShiftVersion != "" {
		d.setupOpenShiftRepositories(ctx, versionInfo)
	}

	// Enable EUS repositories for supported versions
	d.setupEUSRepositories(ctx, versionInfo)

	// Install kernel packages based on kernel type
	if err := d.installKernelPackages(ctx, kernelVersion, versionInfo); err != nil {
		return fmt.Errorf("failed to install kernel packages: %w", err)
	}

	// Install additional dependencies
	if err := d.installRedHatDependencies(ctx, versionInfo); err != nil {
		return fmt.Errorf("failed to install RedHat dependencies: %w", err)
	}

	return nil
}

// buildDriverFromSource builds the driver from source using install.pl
func (d *driverMgr) buildDriverFromSource(ctx context.Context, driverPath, kernelVersion, osType string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Building driver from source", "path", driverPath, "kernel", kernelVersion, "os", osType)

	// Set build flags based on OS type
	buildFlags := d.getBuildFlagsForOS(osType, kernelVersion)

	// Get package suffix based on OS type
	pkgSuffix := d.getPackageSuffix(osType)

	// Get additional build flags based on environment variables
	appendFlags := d.getAppendDriverBuildFlags(osType)

	// Construct install.pl command
	installScript := filepath.Join(driverPath, "install.pl")
	args := []string{
		installScript,
		"--without-depcheck",
		"--kernel", kernelVersion,
		"--kernel-only",
		"--build-only",
		"--with-mlnx-tools",
		"--without-knem" + pkgSuffix,
		"--without-iser" + pkgSuffix,
		"--without-isert" + pkgSuffix,
		"--without-srp" + pkgSuffix,
		"--without-kernel-mft" + pkgSuffix,
		"--without-mlnx-rdma-rxe" + pkgSuffix,
	}

	// Add OS-specific flags
	args = append(args, buildFlags...)

	// Add additional flags based on environment variables
	args = append(args, appendFlags...)

	// Execute the build
	_, _, err := d.cmd.RunCommand(ctx, args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("failed to build driver from source: %w", err)
	}

	log.Info("Driver build completed successfully")
	return nil
}

// getBuildFlagsForOS returns OS-specific build flags
func (d *driverMgr) getBuildFlagsForOS(osType, kernelVersion string) []string {
	switch osType {
	case constants.OSTypeUbuntu:
		return []string{"--disable-kmp", "--without-dkms"}
	case constants.OSTypeSLES:
		return []string{"--disable-kmp", "--without-dkms", "--kernel-sources", "/lib/modules/" + kernelVersion + "/build"}
	case constants.OSTypeRedHat:
		return []string{"--disable-kmp", "--without-dkms"}
	default:
		return []string{}
	}
}

// copyBuildArtifacts copies build artifacts to inventory directory
func (d *driverMgr) copyBuildArtifacts(ctx context.Context, driverPath, inventoryPath, osType string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Copying build artifacts", "from", driverPath, "to", inventoryPath)

	// Determine source and destination paths based on OS type
	var sourcePath string
	var packageType string

	// Get architecture for path construction
	arch := d.getArchitecture(ctx)
	log.V(1).Info("Using architecture for path construction", "arch", arch)

	switch osType {
	case constants.OSTypeUbuntu:
		sourcePath = filepath.Join(driverPath, "DEBS", "ubuntu*", arch, "*.deb")
		packageType = "deb"
	case constants.OSTypeSLES, constants.OSTypeRedHat, constants.OSTypeOpenShift:
		sourcePath = filepath.Join(driverPath, "RPMS", "*", arch, "*.rpm")
		packageType = "rpm"
	default:
		return fmt.Errorf("unsupported OS type for artifact copying: %s", osType)
	}

	log.V(1).Info("Constructed source path", "sourcePath", sourcePath, "packageType", packageType)

	// Copy packages to inventory directory using shell to expand wildcards
	cpCmd := fmt.Sprintf("cp %s %s/", sourcePath, inventoryPath)
	log.V(1).Info("Executing copy command", "command", cpCmd)

	// Debug: List source directory to see what files exist
	lsCmd := fmt.Sprintf("ls -la %s", filepath.Dir(sourcePath))
	log.V(1).Info("Listing source directory", "command", lsCmd)
	_, _, lsErr := d.cmd.RunCommand(ctx, "sh", "-c", lsCmd)
	if lsErr != nil {
		log.V(1).Info("Failed to list source directory", "error", lsErr)
	}

	// Debug: Try to find files matching the pattern
	findCmd := fmt.Sprintf("find %s -name '*.deb' 2>/dev/null || echo 'No .deb files found'", filepath.Join(driverPath, "DEBS"))
	log.V(1).Info("Searching for .deb files", "command", findCmd)
	_, findOutput, findErr := d.cmd.RunCommand(ctx, "sh", "-c", findCmd)
	if findErr != nil {
		log.V(1).Info("Failed to search for .deb files", "error", findErr)
	} else {
		log.V(1).Info("Found .deb files", "output", findOutput)
	}

	// Debug: Check if destination directory exists
	destExistsCmd := fmt.Sprintf("ls -la %s", inventoryPath)
	log.V(1).Info("Checking destination directory", "command", destExistsCmd)
	_, _, destErr := d.cmd.RunCommand(ctx, "sh", "-c", destExistsCmd)
	if destErr != nil {
		log.V(1).Info("Destination directory check failed", "error", destErr)
	}

	_, _, err := d.cmd.RunCommand(ctx, "sh", "-c", cpCmd)
	if err != nil {
		return fmt.Errorf("failed to copy %s packages to inventory: %w", packageType, err)
	}

	log.V(1).Info("Build artifacts copied successfully", "type", packageType)
	return nil
}

// calculateDriverInventoryChecksum calculates MD5 checksum of driver inventory
func (d *driverMgr) calculateDriverInventoryChecksum(ctx context.Context, inventoryPath string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Calculating driver inventory checksum", "path", inventoryPath)

	// Use find and md5sum to calculate checksum through shell to handle pipe
	checksumCmd := fmt.Sprintf("find %s -type f -exec md5sum {} + | md5sum", inventoryPath)
	log.V(1).Info("Executing checksum calculation", "command", checksumCmd)
	stdout, _, err := d.cmd.RunCommand(ctx, "sh", "-c", checksumCmd)
	if err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w", err)
	}

	log.V(1).Info("Checksum calculation output", "output", stdout)

	// Extract checksum from output
	parts := strings.Fields(stdout)
	if len(parts) == 0 {
		return "", fmt.Errorf("no checksum found in output")
	}

	return parts[0], nil
}

// storeBuildChecksum stores the build checksum
func (d *driverMgr) storeBuildChecksum(ctx context.Context, inventoryPath, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)

	checksumPath := filepath.Join(d.cfg.NvidiaNicDriversInventoryPath, kernelVersion, d.cfg.NvidiaNicDriverVer+".checksum")

	// Calculate current checksum
	checksum, err := d.calculateDriverInventoryChecksum(ctx, inventoryPath)
	if err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	// Write checksum to file
	err = d.os.WriteFile(checksumPath, []byte(checksum), 0o644)
	if err != nil {
		return fmt.Errorf("failed to write checksum file: %w", err)
	}

	log.V(1).Info("Stored build checksum", "path", checksumPath, "checksum", checksum)
	return nil
}

// fixSourceLink fixes the /usr/src/ofa_kernel/default symlink
func (d *driverMgr) fixSourceLink(ctx context.Context, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Fixing source link", "kernel", kernelVersion)

	// Check if the symlink exists and points to the correct location
	targetPath := "/usr/src/ofa_kernel/default"
	expectedTarget := filepath.Join("/usr/src/ofa_kernel", d.getArchitecture(ctx), kernelVersion)

	// Read current symlink target
	linkTarget, err := d.os.Readlink(targetPath)
	if err != nil {
		log.V(1).Info("Source link does not exist or is not a symlink", "error", err)
		return nil
	}

	// Check if it's an absolute path and points to the correct location
	if strings.HasPrefix(linkTarget, "/") && linkTarget != expectedTarget {
		// Update the symlink
		_, _, err = d.cmd.RunCommand(ctx, "ln", "-snf", expectedTarget, targetPath)
		if err != nil {
			return fmt.Errorf("failed to update source link: %w", err)
		}
		log.V(1).Info("Updated source link", "from", linkTarget, "to", expectedTarget)
	}

	return nil
}

// getArchitecture returns the system architecture
func (d *driverMgr) getArchitecture(ctx context.Context) string {
	// Execute uname -m to get the machine architecture
	// This matches the bash script: ARCH=$(uname -m)
	output, _, err := d.cmd.RunCommand(ctx, "uname", "-m")
	if err != nil {
		// Fallback to x86_64 if uname fails
		return "x86_64"
	}

	// Trim whitespace and return the architecture
	return strings.TrimSpace(output)
}

// installDriver installs the driver packages from the inventory directory
func (d *driverMgr) installDriver(ctx context.Context, inventoryPath, kernelVersion, osType string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Installing driver packages", "path", inventoryPath, "kernel", kernelVersion, "os", osType)

	// Prevent depmod from giving a WARNING about missing files during installation
	kernelModulesDir := filepath.Join("/lib/modules", kernelVersion)
	if _, err := d.os.Stat(kernelModulesDir); os.IsNotExist(err) {
		log.V(1).Info("Creating kernel modules directory", "path", kernelModulesDir)
		_, _, err := d.cmd.RunCommand(ctx, "mkdir", "-p", kernelModulesDir)
		if err != nil {
			return fmt.Errorf("failed to create kernel modules directory: %w", err)
		}
	}

	// Create required files to prevent depmod warnings
	modulesOrderPath := filepath.Join(kernelModulesDir, "modules.order")
	modulesBuiltinPath := filepath.Join(kernelModulesDir, "modules.builtin")

	log.V(1).Info("Creating modules.order and modules.builtin files")
	_, _, err := d.cmd.RunCommand(ctx, "touch", modulesOrderPath)
	if err != nil {
		return fmt.Errorf("failed to create modules.order file: %w", err)
	}

	_, _, err = d.cmd.RunCommand(ctx, "touch", modulesBuiltinPath)
	if err != nil {
		return fmt.Errorf("failed to create modules.builtin file: %w", err)
	}

	// Install packages based on OS type
	switch osType {
	case constants.OSTypeUbuntu:
		return d.installUbuntuDriver(ctx, inventoryPath, kernelVersion)
	case constants.OSTypeSLES, constants.OSTypeRedHat, constants.OSTypeOpenShift:
		return d.installRedHatDriver(ctx, inventoryPath, kernelVersion)
	default:
		return fmt.Errorf("unsupported OS type for driver installation: %s", osType)
	}
}

// installUbuntuDriver installs driver packages on Ubuntu
func (d *driverMgr) installUbuntuDriver(ctx context.Context, inventoryPath, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Installing Ubuntu driver packages", "path", inventoryPath)

	// Try to install linux-modules-extra package if available
	modulesExtraPkg := fmt.Sprintf("linux-modules-extra-%s", kernelVersion)
	log.V(1).Info("Attempting to install modules extra package", "package", modulesExtraPkg)

	// Update package list and try to install modules-extra package
	_, _, err := d.cmd.RunCommand(ctx, "apt-get", "update")
	if err != nil {
		log.V(1).Info("Failed to update apt packages, continuing", "error", err)
	}

	// Check if the package exists and install it if available
	cmdStr := fmt.Sprintf("LC_ALL=C apt-cache show %s | grep %s && apt-get install -y %s || true",
		modulesExtraPkg, modulesExtraPkg, modulesExtraPkg)
	_, _, err = d.cmd.RunCommand(ctx, "sh", "-c", cmdStr)
	if err != nil {
		log.V(1).Info("Failed to install modules extra package, continuing", "error", err)
	}

	// Install driver packages using shell to expand wildcards
	installCmd := fmt.Sprintf("apt-get install -y %s/*.deb", inventoryPath)
	_, _, err = d.cmd.RunCommand(ctx, "sh", "-c", installCmd)
	if err != nil {
		return fmt.Errorf("failed to install Ubuntu driver packages: %w", err)
	}

	// Run depmod to introduce installed kernel modules
	_, _, err = d.cmd.RunCommand(ctx, "depmod", kernelVersion)
	if err != nil {
		return fmt.Errorf("failed to run depmod: %w", err)
	}

	log.V(1).Info("Ubuntu driver packages installed successfully")
	return nil
}

// installRedHatDriver installs driver packages on RedHat-based systems
func (d *driverMgr) installRedHatDriver(ctx context.Context, inventoryPath, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Installing RedHat driver packages", "path", inventoryPath)

	// Install driver packages using rpm
	_, _, err := d.cmd.RunCommand(ctx, "rpm", "-ivh", "--replacepkgs", "--nodeps", filepath.Join(inventoryPath, "*.rpm"))
	if err != nil {
		return fmt.Errorf("failed to install RedHat driver packages: %w", err)
	}

	// Run depmod to introduce installed kernel modules
	_, _, err = d.cmd.RunCommand(ctx, "depmod", kernelVersion)
	if err != nil {
		return fmt.Errorf("failed to run depmod: %w", err)
	}

	log.V(1).Info("RedHat driver packages installed successfully")
	return nil
}

// ubuntuSyncNetworkConfigurationTools handles Ubuntu-specific network configuration tool synchronization
func (d *driverMgr) ubuntuSyncNetworkConfigurationTools(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Syncing Ubuntu network configuration tools")

	// Check if /etc/network/interfaces exists
	interfacesPath := "/etc/network/interfaces"
	if _, err := d.os.Stat(interfacesPath); os.IsNotExist(err) {
		log.V(1).Info("/etc/network/interfaces not found, renaming ifup file to prevent issues with mlnx_interface_mgr.sh")

		// Check if /sbin/ifup exists and rename it to /sbin/ifup.bk
		ifupPath := "/sbin/ifup"
		if _, err := d.os.Stat(ifupPath); err == nil {
			_, _, err := d.cmd.RunCommand(ctx, "mv", ifupPath, ifupPath+".bk")
			if err != nil {
				return fmt.Errorf("failed to rename ifup file: %w", err)
			}
			log.V(1).Info("Renamed ifup file to prevent mlnx_interface_mgr.sh from reading missing /etc/network/interfaces")
		}
	} else if err != nil {
		return fmt.Errorf("failed to check /etc/network/interfaces: %w", err)
	}

	log.V(1).Info("Ubuntu network configuration tools sync completed")
	return nil
}

// getPackageSuffix returns the package suffix based on OS type
func (d *driverMgr) getPackageSuffix(osType string) string {
	switch osType {
	case constants.OSTypeUbuntu:
		return "-modules"
	case constants.OSTypeSLES, constants.OSTypeRedHat, constants.OSTypeOpenShift:
		return ""
	default:
		return ""
	}
}

// getAppendDriverBuildFlags returns additional build flags based on configuration
func (d *driverMgr) getAppendDriverBuildFlags(osType string) []string {
	// If ENABLE_NFSRDMA is false, add additional flags
	if !d.cfg.EnableNfsRdma {
		pkgSuffix := d.getPackageSuffix(osType)
		return []string{
			"--without-mlnx-nfsrdma" + pkgSuffix,
			"--without-mlnx-nvme" + pkgSuffix,
		}
	}

	return []string{}
}

// setupOpenShiftRepositories configures OpenShift-specific repositories
func (d *driverMgr) setupOpenShiftRepositories(ctx context.Context, versionInfo *host.RedhatVersionInfo) {
	log := logr.FromContextOrDiscard(ctx)
	arch := d.getArchitecture(ctx)

	log.V(1).Info("Setting up OpenShift repositories",
		"version", versionInfo.OpenShiftVersion,
		"major", versionInfo.MajorVersion,
		"arch", arch)

	// Enable RHOCP repository
	repoName := fmt.Sprintf("rhocp-%s-for-rhel-%d-%s-rpms", versionInfo.OpenShiftVersion, versionInfo.MajorVersion, arch)
	_, _, err := d.cmd.RunCommand(ctx, "dnf", "config-manager", "--set-enabled", repoName)
	if err != nil {
		log.V(1).Info("Failed to enable RHOCP repository, continuing", "repo", repoName, "error", err)
	}

	// Test if makecache works
	_, _, err = d.cmd.RunCommand(ctx, "dnf", "makecache", "--releasever="+versionInfo.FullVersion)
	if err != nil {
		log.V(1).Info("Makecache failed, disabling RHOCP repository", "error", err)
		_, _, _ = d.cmd.RunCommand(ctx, "dnf", "config-manager", "--set-disabled", repoName)
	}
}

// setupEUSRepositories configures EUS (Extended Update Support) repositories for supported versions
func (d *driverMgr) setupEUSRepositories(ctx context.Context, versionInfo *host.RedhatVersionInfo) {
	log := logr.FromContextOrDiscard(ctx)
	arch := d.getArchitecture(ctx)

	// EUS is available for specific versions
	eusVersions := []string{"8.4", "8.6", "8.8", "9.0", "9.2", "9.4"}

	for _, version := range eusVersions {
		if versionInfo.FullVersion == version {
			log.V(1).Info("Enabling EUS repository", "version", version, "arch", arch)
			repoName := fmt.Sprintf("rhel-%d-for-%s-baseos-eus-rpms", versionInfo.MajorVersion, arch)
			_, _, err := d.cmd.RunCommand(ctx, "dnf", "config-manager", "--set-enabled", repoName)
			if err != nil {
				log.V(1).Info("Failed to enable EUS repository", "repo", repoName, "error", err)
			}
			break
		}
	}
}

// installKernelPackages installs kernel packages based on kernel type
func (d *driverMgr) installKernelPackages(ctx context.Context, kernelVersion string, versionInfo *host.RedhatVersionInfo) error {
	log := logr.FromContextOrDiscard(ctx)

	// Determine kernel type and naming pattern
	kernelType, kVer, rtHpSubstr, releaseverStr := d.analyzeKernelType(ctx, kernelVersion, versionInfo)

	log.V(1).Info("Installing kernel packages", "type", kernelType, "version", kVer, "rtHpSubstr", rtHpSubstr)

	// Handle RT and 64k kernels that need special repo setup
	if kernelType == kernelTypeRT || kernelType == kernelType64k {
		if err := d.setupSpecialKernelRepos(ctx); err != nil {
			return fmt.Errorf("failed to setup special kernel repositories: %w", err)
		}
	}

	// Install standard kernel packages for non-RT, non-64k kernels
	if kernelType == kernelTypeStandard {
		packages := []string{
			"kernel-" + kernelVersion,
			"kernel-headers-" + kernelVersion,
			"kernel-core-" + kernelVersion,
		}

		for _, pkg := range packages {
			args := []string{"dnf", "-q", "-y"}
			if releaseverStr != "" {
				args = append(args, releaseverStr)
			}
			args = append(args, "install", pkg)

			_, _, err := d.cmd.RunCommand(ctx, args[0], args[1:]...)
			if err != nil {
				return fmt.Errorf("failed to install %s: %w", pkg, err)
			}
		}

		// Install kernel-devel with --allowerasing flag
		args := []string{"dnf", "-q", "-y"}
		if releaseverStr != "" {
			args = append(args, releaseverStr)
		}
		args = append(args, "install", "kernel-devel-"+kernelVersion, "--allowerasing")

		_, _, err := d.cmd.RunCommand(ctx, args[0], args[1:]...)
		if err != nil {
			return fmt.Errorf("failed to install kernel-devel: %w", err)
		}
	}

	// Install kernel development and modules packages
	args := []string{"dnf", "-q", "-y"}
	if releaseverStr != "" {
		args = append(args, releaseverStr)
	}
	args = append(args, "install", "kernel-"+rtHpSubstr+"devel-"+kVer, "kernel-"+rtHpSubstr+"modules-"+kVer)

	_, _, err := d.cmd.RunCommand(ctx, args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("failed to install kernel development packages: %w", err)
	}

	return nil
}

// analyzeKernelType analyzes the kernel version to determine type and naming pattern
func (d *driverMgr) analyzeKernelType(
	ctx context.Context,
	kernelVersion string,
	versionInfo *host.RedhatVersionInfo,
) (string, string, string, string) {
	rtHpSubstr := ""
	kVer := kernelVersion
	releaseverStr := "--releasever=" + versionInfo.FullVersion

	// Check for RT kernel
	if strings.Contains(kernelVersion, "rt") {
		releaseverStr = ""
		rtHpSubstr = "rt-"

		// Handle different RT kernel naming patterns
		if strings.HasSuffix(kernelVersion, "rt") {
			// RH9.X RT kernel pattern: 5.14.0-362.13.1.el9_3.x86_64+rt
			kVer = strings.TrimSuffix(kernelVersion, ".x86_64") + "." + d.getArchitecture(ctx)
		} else {
			// RH8.X RT kernel pattern: 4.18.0-513.11.1.rt7.313.el8_9.x86_64
			kVer = kernelVersion
		}
		return kernelTypeRT, kVer, rtHpSubstr, releaseverStr
	}

	// Check for 64k page size kernel
	if strings.Contains(kernelVersion, "64k") {
		releaseverStr = ""
		rtHpSubstr = "64k-"

		if strings.HasSuffix(kernelVersion, "64k") {
			kVer = strings.TrimSuffix(kernelVersion, ".x86_64") + "." + d.getArchitecture(ctx)
		}
		return kernelType64k, kVer, rtHpSubstr, releaseverStr
	}

	return kernelTypeStandard, kVer, rtHpSubstr, releaseverStr
}

// checkLoadedKmodSrcverVsModinfo checks if loaded kernel module srcversion matches modinfo
func (d *driverMgr) checkLoadedKmodSrcverVsModinfo(ctx context.Context, modules []string) (bool, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Get list of loaded modules using host interface
	loadedModules, err := d.host.LsMod(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get loaded modules: %w", err)
	}

	for _, module := range modules {
		log.V(1).Info("Checking module", "module", module)

		// Check if module is loaded
		if _, exists := loadedModules[module]; !exists {
			log.V(1).Info("Module not loaded", "module", module)
			return false, nil // Module not loaded, need to reload
		}

		// Get srcversion from modinfo
		srcverFromModinfo, _, err := d.cmd.RunCommand(ctx, "modinfo", module)
		if err != nil {
			log.V(1).Info("Failed to get modinfo for module", "module", module, "error", err)
			return false, nil // Module not found, need to reload
		}

		// Extract srcversion from modinfo output
		srcverFromModinfo = strings.TrimSpace(srcverFromModinfo)
		lines := strings.Split(srcverFromModinfo, "\n")
		var modinfoSrcver string
		for _, line := range lines {
			if strings.Contains(line, "srcversion") {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					modinfoSrcver = parts[len(parts)-1]
					break
				}
			}
		}

		// Get srcversion from sysfs
		sysfsPath := fmt.Sprintf("/sys/module/%s/srcversion", module)
		srcverFromSysfs, _, err := d.cmd.RunCommand(ctx, "cat", sysfsPath)
		if err != nil {
			log.V(1).Info("Failed to read sysfs srcversion for module", "module", module, "error", err)
			return false, nil // Module not loaded, need to reload
		}

		srcverFromSysfs = strings.TrimSpace(srcverFromSysfs)

		log.V(1).Info("Module version check", "module", module, "modinfo", modinfoSrcver, "sysfs", srcverFromSysfs)

		if modinfoSrcver != srcverFromSysfs {
			log.V(1).Info("Module srcversion differs", "module", module)
			return false, nil
		}
	}

	return true, nil
}

// restartDriver restarts the driver modules
func (d *driverMgr) restartDriver(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Restarting driver modules")

	// Load dependencies
	_, _, err := d.cmd.RunCommand(ctx, "modprobe", "-d", "/host", "tls")
	if err != nil {
		log.V(1).Info("Failed to load tls module", "error", err)
		// Non-fatal, continue
	}

	_, _, err = d.cmd.RunCommand(ctx, "modprobe", "-d", "/host", "psample")
	if err != nil {
		log.V(1).Info("Failed to load psample module", "error", err)
		// Non-fatal, continue
	}

	// Check if mlx5_ib depends on macsec and load it if needed
	depends, _, err := d.cmd.RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib")
	if err == nil && strings.Contains(depends, "macsec") {
		_, _, err = d.cmd.RunCommand(ctx, "modprobe", "-d", "/host", "macsec")
		if err != nil {
			log.V(1).Info("Failed to load macsec module", "error", err)
			// Non-fatal, continue
		}
	}

	// Load pci-hyperv-intf if needed (simplified logic)
	arch := d.getArchitecture(ctx)
	if arch != "aarch64" {
		_, _, err = d.cmd.RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf")
		if err != nil {
			log.V(1).Info("Failed to load pci-hyperv-intf module", "error", err)
			// Non-fatal, continue
		}
	}

	// Unload storage modules if enabled
	if d.cfg.UnloadStorageModules {
		if err := d.unloadStorageModules(ctx); err != nil {
			log.V(1).Info("Failed to unload storage modules", "error", err)
			// Non-fatal, continue
		}
	}

	// Restart openibd service
	_, _, err = d.cmd.RunCommand(ctx, "/etc/init.d/openibd", "restart")
	if err != nil {
		return fmt.Errorf("failed to restart openibd service: %w", err)
	}

	// Load mlx5_vdpa if available
	_, _, err = d.cmd.RunCommand(ctx, "modinfo", "mlx5_vdpa")
	if err == nil {
		// Module exists, try to load it
		_, _, err = d.cmd.RunCommand(ctx, "modprobe", "mlx5_vdpa")
		if err != nil {
			log.V(1).Info("Failed to load mlx5_vdpa module", "error", err)
			// Non-fatal, continue
		}
	} else {
		log.V(1).Info("mlx5_vdpa module not found, skipping")
	}

	return nil
}

// loadNfsRdma loads NFS RDMA modules if enabled
func (d *driverMgr) loadNfsRdma(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	if !d.cfg.EnableNfsRdma {
		return nil
	}

	log.V(1).Info("Loading NFS RDMA modules")

	_, _, err := d.cmd.RunCommand(ctx, "modprobe", "rpcrdma")
	if err != nil {
		return fmt.Errorf("failed to load rpcrdma module: %w", err)
	}

	return nil
}

// printLoadedDriverVersion prints the currently loaded driver version
func (d *driverMgr) printLoadedDriverVersion(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	// Check if mlx5_core is loaded using host interface
	loadedModules, err := d.host.LsMod(ctx)
	if err != nil {
		return fmt.Errorf("failed to check loaded modules: %w", err)
	}

	// Check if mlx5_core is loaded
	if _, exists := loadedModules["mlx5_core"]; !exists {
		log.V(1).Info("mlx5_core module not loaded")
		return nil
	}

	// Get first Mellanox network device name
	netdevName, err := d.getFirstMlxNetdevName(ctx)
	if err != nil {
		log.V(1).Info("No Mellanox network device found", "error", err)
		return nil
	}

	// Get driver version via ethtool
	ethtoolOutput, _, err := d.cmd.RunCommand(ctx, "ethtool", "--driver", netdevName)
	if err != nil {
		log.V(1).Info("Failed to get driver version via ethtool", "error", err)
		return nil
	}

	// Extract version from ethtool output
	lines := strings.Split(ethtoolOutput, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "version:") {
			version := strings.TrimSpace(strings.TrimPrefix(line, "version:"))
			log.Info("Current mlx5_core driver version", "version", version)
			break
		}
	}

	return nil
}

// getFirstMlxNetdevName gets the first Mellanox network device name
func (d *driverMgr) getFirstMlxNetdevName(ctx context.Context) (string, error) {
	// List network devices
	netdevOutput, _, err := d.cmd.RunCommand(ctx, "ls", "/sys/class/net/")
	if err != nil {
		return "", fmt.Errorf("failed to list network devices: %w", err)
	}

	devices := strings.Fields(netdevOutput)
	for _, device := range devices {
		// Check if this is a Mellanox device by looking at driver
		driverPath := fmt.Sprintf("/sys/class/net/%s/device/driver", device)
		driverLink, _, err := d.cmd.RunCommand(ctx, "readlink", driverPath)
		if err != nil {
			continue
		}

		if strings.Contains(driverLink, "mlx5") {
			return device, nil
		}
	}

	return "", fmt.Errorf("no Mellanox network device found")
}

// unloadStorageModules modifies the openibd script to include storage modules in the unload list
func (d *driverMgr) unloadStorageModules(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Unloading storage modules")

	// Determine the unload storage script path
	unloadStorageScript := "/etc/init.d/openibd"
	if _, err := d.os.Stat("/usr/share/mlnx_ofed/mod_load_funcs"); err == nil {
		unloadStorageScript = "/usr/share/mlnx_ofed/mod_load_funcs"
	}

	log.V(1).Info("Using unload storage script", "script", unloadStorageScript)

	// Create the sed command to add storage modules to UNLOAD_MODULES
	// This matches the bash script:
	// sed -i -e '/^[[:space:]]*UNLOAD_MODULES="[a-z]/a\    UNLOAD_MODULES="$UNLOAD_MODULES \
	// ib_isert nvme_rdma nvmet_rdma rpcrdma xprtrdma ib_srpt"'
	storageModulesStr := strings.Join(d.cfg.StorageModules, " ")
	sedCommand := fmt.Sprintf(`/^[[:space:]]*UNLOAD_MODULES="[a-z]/a\    UNLOAD_MODULES="$UNLOAD_MODULES %s"`, storageModulesStr)
	log.V(1).Info("Executing sed command", "sedCommand", sedCommand, "storageModules", d.cfg.StorageModules)

	// Execute sed command to modify the script
	_, _, err := d.cmd.RunCommand(ctx, "sed", "-i", "-e", sedCommand, unloadStorageScript)
	if err != nil {
		return fmt.Errorf("failed to modify unload storage script: %w", err)
	}

	// Verify the modification was successful by checking if storage modules are now in the script
	// This matches the bash script: if [ `grep ib_isert ${unload_storage_script} -c` -lt 1 ]; then
	grepCmd := fmt.Sprintf("grep %s %s -c", d.cfg.StorageModules[0], unloadStorageScript)
	_, stdout, err := d.cmd.RunCommand(ctx, "sh", "-c", grepCmd)
	if err != nil {
		return fmt.Errorf("failed to verify storage modules injection: %w", err)
	}

	count := strings.TrimSpace(stdout)
	log.V(1).Info("Verification result", "grepCmd", grepCmd, "count", count)

	if count == "0" {
		return fmt.Errorf("failed to inject storage modules for unload")
	}

	log.V(1).Info("Successfully added storage modules to unload script", "modules", d.cfg.StorageModules)
	return nil
}

// setupSpecialKernelRepos sets up repositories for RT and 64k kernels
func (d *driverMgr) setupSpecialKernelRepos(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Setting up special kernel repositories")

	// Copy redhat.repo from host
	_, _, err := d.cmd.RunCommand(ctx, "cp", "/host/etc/yum.repos.d/redhat.repo", "/etc/yum.repos.d/")
	if err != nil {
		return fmt.Errorf("failed to copy redhat.repo: %w", err)
	}

	return nil
}

// installRedHatDependencies installs additional RedHat dependencies
func (d *driverMgr) installRedHatDependencies(ctx context.Context, versionInfo *host.RedhatVersionInfo) error {
	log := logr.FromContextOrDiscard(ctx)

	log.V(1).Info("Installing RedHat dependencies")

	// Install additional dependencies
	packages := []string{
		"elfutils-libelf-devel",
		"kernel-rpm-macros",
		"numactl-libs",
		"lsof",
		"rpm-build",
		"patch",
		"hostname",
	}

	args := []string{"dnf", "-q", "-y", "--releasever=" + versionInfo.FullVersion, "install"}
	args = append(args, packages...)

	_, _, err := d.cmd.RunCommand(ctx, args[0], args[1:]...)
	if err != nil {
		return fmt.Errorf("failed to install RedHat dependencies: %w", err)
	}

	// Test makecache and disable EUS if it fails
	_, _, err = d.cmd.RunCommand(ctx, "dnf", "makecache", "--releasever="+versionInfo.FullVersion)
	if err != nil {
		log.V(1).Info("Makecache failed, disabling EUS repository", "error", err)
		arch := d.getArchitecture(ctx)
		repoName := fmt.Sprintf("rhel-%d-for-%s-baseos-eus-rpms", versionInfo.MajorVersion, arch)
		_, _, _ = d.cmd.RunCommand(ctx, "dnf", "config-manager", "--set-disabled", repoName)
	}

	return nil
}

// updateCACertificates updates system CA certificates for supported OS types
func (d *driverMgr) updateCACertificates(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)

	// Constants for CA certificate update commands
	const updateCaCertificatesCmd = "update-ca-certificates"
	const updateCaTrustCmd = "update-ca-trust extract"

	// Get OS type to determine the appropriate CA certificate update command
	osType, err := d.host.GetOSType(ctx)
	if err != nil {
		return fmt.Errorf("failed to get OS type: %w", err)
	}

	// Determine the command and log message based on OS type
	var command string
	var logMessage string

	switch osType {
	case constants.OSTypeUbuntu:
		command = updateCaCertificatesCmd
		logMessage = "Updating system CA certificates (Ubuntu)..."
	case constants.OSTypeSLES:
		command = updateCaCertificatesCmd
		logMessage = "Updating system CA certificates (SLES)..."
	case constants.OSTypeRedHat, constants.OSTypeOpenShift:
		command = updateCaTrustCmd
		logMessage = "Updating system CA certificates (RHEL/OpenShift)..."
	default:
		log.V(1).Info("Skipping CA certificate update for unsupported OS", "os", osType)
		return nil
	}

	log.Info(logMessage)

	// Extract the base command for existence check (remove arguments)
	baseCommand := strings.Fields(command)[0]

	// Check if the command exists using shell with 'command -v'
	_, _, err = d.cmd.RunCommand(ctx, "sh", "-c", "command -v "+baseCommand)
	if err != nil {
		log.Info("[WARN] CA certificate update command not found", "command", baseCommand)
		// Command not found is not a fatal error, continue execution
		return nil //nolint:nilerr // Intentionally ignoring error - command not found is not fatal
	}

	// Run the appropriate command with || true to ignore errors
	// This matches the bash script pattern: exec_cmd "command || true"
	_, _, err = d.cmd.RunCommand(ctx, "sh", "-c", command+" || true")
	if err != nil {
		log.V(1).Info("CA certificate update command failed", "command", command, "error", err)
		// Non-fatal error, continue
	}

	log.V(1).Info("CA certificate update completed", "os", osType)
	return nil
}
