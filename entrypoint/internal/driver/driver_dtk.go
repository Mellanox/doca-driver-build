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
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
)

// buildDriverDTK orchestrates the driver build using the OpenShift Driver Toolkit (DTK)
func (d *driverMgr) buildDriverDTK(ctx context.Context, kernelVersion, inventoryPath string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Starting DTK driver build")

	// Sanitize kernel version for DTK shared directory
	// Matches bash: DTK_KVER=$(echo "${FULL_KVER}" | sed 's/[^-A-Za-z0-9_.]/_/g' | sed 's/^[-_.]*//;s/[-_.]*$//')
	dtkKver := sanitizeKernelVersion(kernelVersion)
	dtkSharedDir := filepath.Join(d.cfg.DtkOcpNicSharedDir, dtkKver)

	// Construct done flag path
	// Matches bash: DTK_OCP_DONE_COMPILE_FLAG="${DTK_OCP_DONE_COMPILE_FLAG_PREFIX}$(echo ${NVIDIA_NIC_DRIVER_VER} | sed 's/[.-]/_/g')"
	verSanitized := strings.ReplaceAll(strings.ReplaceAll(d.cfg.NvidiaNicDriverVer, ".", "_"), "-", "_")
	doneFlagName := constants.DtkDoneCompileFlagPrefix + verSanitized
	doneFlagPath := filepath.Join(dtkSharedDir, doneFlagName)
	startFlagPath := filepath.Join(dtkSharedDir, constants.DtkStartCompileFlag)

	// Check if build is already done
	if _, err := d.os.Stat(doneFlagPath); os.IsNotExist(err) {
		log.Info("DTK build not done, setting up build")

		if err := d.dtkSetupDriverBuild(ctx, dtkSharedDir, startFlagPath, doneFlagPath); err != nil {
			return fmt.Errorf("failed to setup DTK build: %w", err)
		}

		if err := d.dtkWaitForBuild(ctx, doneFlagPath); err != nil {
			return fmt.Errorf("failed waiting for DTK build: %w", err)
		}
	} else {
		log.Info("DTK build already done", "flag", doneFlagPath)
	}

	// Finalize build (copy artifacts)
	if err := d.dtkFinalizeDriverBuild(ctx, dtkSharedDir, inventoryPath); err != nil {
		return fmt.Errorf("failed to finalize DTK build: %w", err)
	}

	return nil
}

// sanitizeKernelVersion sanitizes the kernel version string for use in directory names
func sanitizeKernelVersion(version string) string {
	// Replace all non-alphanumeric characters (except -._) with underscore
	reg := regexp.MustCompile(`[^-A-Za-z0-9_.]`)
	sanitized := reg.ReplaceAllString(version, "_")
	// Trim leading/trailing -._
	return strings.Trim(sanitized, "-._")
}

// dtkSetupDriverBuild prepares the shared directory and script for DTK build
func (d *driverMgr) dtkSetupDriverBuild(ctx context.Context, sharedDir, startFlagPath, doneFlagPath string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Setting up DTK driver build", "sharedDir", sharedDir)

	// Create shared directory
	if err := d.os.MkdirAll(sharedDir, 0o755); err != nil {
		return fmt.Errorf("failed to create shared directory: %w", err)
	}

	// Copy driver sources to shared directory
	// Matches bash: cp -r ${NVIDIA_NIC_DRIVER_PATH} ${DTK_OCP_NIC_SHARED_DIR}/
	srcDir := d.cfg.NvidiaNicDriverPath
	// Use expected directory name format to ensure DTK build script finds it
	expectedName := fmt.Sprintf("MLNX_OFED_SRC-%s", d.cfg.NvidiaNicDriverVer)
	destDir := filepath.Join(sharedDir, expectedName)

	// Clean up destination if it exists to avoid nesting (cp -r behavior)
	if err := d.os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("failed to clean up destination directory: %w", err)
	}

	log.Info("Copying driver sources", "from", srcDir, "to", destDir)
	if err := d.copyDir(ctx, srcDir, destDir); err != nil {
		return fmt.Errorf("failed to copy driver sources: %w", err)
	}

	// Copy entrypoint binary to shared directory
	entrypointPath := "/root/entrypoint" // Assumed location based on Dockerfile
	destEntrypointPath := filepath.Join(sharedDir, "entrypoint")
	log.Info("Copying entrypoint binary", "from", entrypointPath, "to", destEntrypointPath)
	// We use copyFile instead of copyDir for a single file
	if _, _, err := d.cmd.RunCommand(ctx, "cp", entrypointPath, destEntrypointPath); err != nil {
		return fmt.Errorf("failed to copy entrypoint binary: %w", err)
	}

	// Create dtk.env file
	// Get append flags
	appendFlags := d.getAppendDriverBuildFlags(constants.OSTypeRedHat)
	appendFlagsStr := strings.Join(appendFlags, " ")

	envContent := fmt.Sprintf(`export DTK_OCP_NIC_SHARED_DIR="%s"
export DTK_OCP_COMPILED_DRIVER_VER="%s"
export DTK_OCP_START_COMPILE_FLAG="%s"
export DTK_OCP_DONE_COMPILE_FLAG="%s"
export APPEND_DRIVER_BUILD_FLAGS="%s"
export USE_NEW_ENTRYPOINT="true"
export NVIDIA_NIC_DRIVER_VER="%s"
`, sharedDir, d.cfg.NvidiaNicDriverVer, startFlagPath, doneFlagPath, appendFlagsStr, d.cfg.NvidiaNicDriverVer)

	envPath := filepath.Join(sharedDir, "dtk.env")
	if err := d.os.WriteFile(envPath, []byte(envContent), 0o644); err != nil {
		return fmt.Errorf("failed to write dtk.env: %w", err)
	}

	// Copy build script (loader)
	srcScriptPath := constants.DtkOcpBuildScriptPath
	destScriptPath := filepath.Join(sharedDir, filepath.Base(srcScriptPath))
	log.Info("Copying build script", "from", srcScriptPath, "to", destScriptPath)
	// We use copyFile equivalent (run command cp)
	if _, _, err := d.cmd.RunCommand(ctx, "cp", srcScriptPath, destScriptPath); err != nil {
		return fmt.Errorf("failed to copy build script: %w", err)
	}

	// Create start flag
	log.Info("Creating start compile flag", "path", startFlagPath)
	if _, err := d.os.Create(startFlagPath); err != nil {
		return fmt.Errorf("failed to create start flag: %w", err)
	}

	return nil
}

// dtkWaitForBuild waits for the DTK build to complete
func (d *driverMgr) dtkWaitForBuild(ctx context.Context, doneFlagPath string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Waiting for DTK build to complete", "doneFlag", doneFlagPath)

	sleepSec := 300
	totalRetries := 10
	totalSleepSec := 0

	for totalRetries > 0 {
		if _, err := d.os.Stat(doneFlagPath); err == nil {
			log.Info("DTK build completed")
			return nil
		}

		log.Info("Awaiting DTK compilation", "next_query_sec", sleepSec)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(sleepSec) * time.Second):
		}

		totalSleepSec += sleepSec
		if sleepSec > 10 {
			sleepSec /= 2
		}
		totalRetries--
	}

	return fmt.Errorf("timeout (%d sec) awaiting DTK compilation, %s not found", totalSleepSec, doneFlagPath)
}

// dtkFinalizeDriverBuild copies the built artifacts back to the inventory
func (d *driverMgr) dtkFinalizeDriverBuild(ctx context.Context, sharedDir, inventoryPath string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Finalizing DTK driver build", "inventoryPath", inventoryPath)

	if err := d.createInventoryDirectory(ctx, inventoryPath); err != nil {
		return err
	}

	// Construct path to RPMs in shared dir
	// Matches bash: rpms_path="${DTK_OCP_NIC_SHARED_DIR}/MLNX_OFED_SRC-${NVIDIA_NIC_DRIVER_VER}/RPMS/redhat-release-*/${ARCH}/"
	arch := d.getArchitecture(ctx)
	srcDirName := fmt.Sprintf("MLNX_OFED_SRC-%s", d.cfg.NvidiaNicDriverVer)
	// We need to handle the wildcard "redhat-release-*"
	rpmsBase := filepath.Join(sharedDir, srcDirName, "RPMS")

	// Find the redhat-release directory
	entries, err := d.os.ReadDir(rpmsBase)
	if err != nil {
		return fmt.Errorf("failed to read RPMS directory %s: %w", rpmsBase, err)
	}

	var redhatDir string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "redhat-release-") {
			redhatDir = entry.Name()
			break
		}
	}

	if redhatDir == "" {
		return fmt.Errorf("redhat-release directory not found in %s", rpmsBase)
	}

	rpmsPath := filepath.Join(rpmsBase, redhatDir, arch)

	// Copy RPMs
	// Matches bash: cp -rf ${rpms_path}/*.rpm ${driver_inventory_path}/
	log.Info("Copying RPMs", "from", rpmsPath, "to", inventoryPath)

	// Copy RPMs using glob to avoid shell injection
	rpmsGlob := filepath.Join(rpmsPath, "*.rpm")
	files, err := filepath.Glob(rpmsGlob)
	if err != nil {
		return fmt.Errorf("failed to glob RPM files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no RPM files found in %s", rpmsPath)
	}

	for _, file := range files {
		dest := filepath.Join(inventoryPath, filepath.Base(file))
		if _, _, err := d.cmd.RunCommand(ctx, "cp", "-f", file, dest); err != nil {
			return fmt.Errorf("failed to copy %s: %w", file, err)
		}
	}

	return nil
}

// copyDir copies a directory recursively
func (d *driverMgr) copyDir(ctx context.Context, src, dest string) error {
	// Using cp -rT to treat dest as a normal file (directory)
	// This ensures contents of src are copied into dest, not src into dest/src
	_, _, err := d.cmd.RunCommand(ctx, "cp", "-rT", src, dest)
	return err
}
