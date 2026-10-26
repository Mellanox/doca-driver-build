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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/dtk"
	hostutils "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
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

	if err := d.discardStaleDoneFlag(ctx, dtkSharedDir, doneFlagPath); err != nil {
		return err
	}

	// Check if build is already done
	if _, err := d.os.Stat(doneFlagPath); os.IsNotExist(err) {
		log.Info("DTK build not done, setting up build")
		if err := d.buildAndWaitDTK(ctx, dtkSharedDir, startFlagPath, doneFlagPath, kernelVersion); err != nil {
			return err
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

// dtkSetupDriverBuild prepares the shared directory and signals the sidecar to compile.
func (d *driverMgr) dtkSetupDriverBuild(ctx context.Context, sharedDir, startFlagPath, doneFlagPath, kernelVersion string) error {
	logr.FromContextOrDiscard(ctx).Info("Setting up DTK driver build", "sharedDir", sharedDir)

	if err := d.os.MkdirAll(sharedDir, 0o755); err != nil {
		return fmt.Errorf("failed to create shared directory: %w", err)
	}
	if err := d.stageDTKDriverSources(ctx, sharedDir); err != nil {
		return err
	}
	if err := d.copyDTKEntrypoint(ctx, sharedDir); err != nil {
		return err
	}
	if err := d.writeDTKEnv(ctx, sharedDir, startFlagPath, doneFlagPath, kernelVersion); err != nil {
		return err
	}
	if err := d.copyDTKBuildScript(ctx, sharedDir); err != nil {
		return err
	}
	return d.writeStartFlag(ctx, startFlagPath)
}

// stageDTKDriverSources replaces the shared source tree. The destination is removed first so
// cp -rT does not nest a previous copy inside itself.
func (d *driverMgr) stageDTKDriverSources(ctx context.Context, sharedDir string) error {
	srcDir := d.cfg.NvidiaNicDriverPath
	destDir := filepath.Join(sharedDir, dtk.SourceDirName(d.cfg.NvidiaNicDriverVer))
	if err := d.os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("failed to clean up destination directory: %w", err)
	}
	logr.FromContextOrDiscard(ctx).Info("Copying driver sources", "from", srcDir, "to", destDir)
	if err := d.copyDir(ctx, srcDir, destDir); err != nil {
		return fmt.Errorf("failed to copy driver sources: %w", err)
	}
	return nil
}

// copyDTKEntrypoint publishes the driver entrypoint for the sidecar. The sidecar may still be
// executing the previous copy after a node reboot restarted the containers in place.
func (d *driverMgr) copyDTKEntrypoint(ctx context.Context, sharedDir string) error {
	const entrypointPath = "/root/entrypoint" // location in the driver image
	dest := filepath.Join(sharedDir, "entrypoint")
	logr.FromContextOrDiscard(ctx).Info("Copying entrypoint binary", "from", entrypointPath, "to", dest)
	if err := d.copyFileReplacing(ctx, entrypointPath, dest); err != nil {
		return fmt.Errorf("failed to copy entrypoint binary: %w", err)
	}
	return nil
}

// writeDTKEnv records the paths and build flags the sidecar reads before it compiles.
func (d *driverMgr) writeDTKEnv(ctx context.Context, sharedDir, startFlagPath, doneFlagPath, kernelVersion string) error {
	appendFlags, err := d.getDTKAppendDriverBuildFlags(ctx, kernelVersion)
	if err != nil {
		return err
	}
	appendFlagsStr := strings.Join(appendFlags, " ")
	envContent := fmt.Sprintf(`export DTK_OCP_NIC_SHARED_DIR="%s"
export DTK_OCP_COMPILED_DRIVER_VER="%s"
export DTK_OCP_START_COMPILE_FLAG="%s"
export DTK_OCP_DONE_COMPILE_FLAG="%s"
export APPEND_DRIVER_BUILD_FLAGS="%s"
export USE_NEW_ENTRYPOINT="true"
export NVIDIA_NIC_DRIVER_VER="%s"
export USE_DKMS="%v"
`, sharedDir, d.cfg.NvidiaNicDriverVer, startFlagPath, doneFlagPath, appendFlagsStr, d.cfg.NvidiaNicDriverVer, d.cfg.UseDKMS)

	if err := d.os.WriteFile(filepath.Join(sharedDir, "dtk.env"), []byte(envContent), 0o644); err != nil {
		return fmt.Errorf("failed to write dtk.env: %w", err)
	}
	return nil
}

// copyDTKBuildScript publishes the sidecar loader script into the shared directory.
func (d *driverMgr) copyDTKBuildScript(ctx context.Context, sharedDir string) error {
	src := constants.DtkOcpBuildScriptPath
	dest := filepath.Join(sharedDir, filepath.Base(src))
	logr.FromContextOrDiscard(ctx).Info("Copying build script", "from", src, "to", dest)
	if err := d.copyFileReplacing(ctx, src, dest); err != nil {
		return fmt.Errorf("failed to copy build script: %w", err)
	}
	return nil
}

// copyFileReplacing copies src to a temporary sibling of dest and renames it over dest. Unlike a
// plain cp, the rename succeeds when dest is a binary that is currently running.
func (d *driverMgr) copyFileReplacing(ctx context.Context, src, dest string) error {
	tmp := dest + ".tmp"
	if _, _, err := d.cmd.RunCommand(ctx, "cp", src, tmp); err != nil {
		return err
	}
	if _, _, err := d.cmd.RunCommand(ctx, "mv", "-f", tmp, dest); err != nil {
		return err
	}
	return nil
}

// discardStaleDoneFlag removes a done flag that has no built RPMs behind it (e.g. written for a
// build interrupted by a node reboot), so the build is redone instead of failing in finalize forever.
func (d *driverMgr) discardStaleDoneFlag(ctx context.Context, sharedDir, doneFlagPath string) error {
	if _, err := d.os.Stat(doneFlagPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to stat done flag: %w", err)
	}
	if dtk.HasBuiltRPMs(filepath.Join(sharedDir, dtk.SourceDirName(d.cfg.NvidiaNicDriverVer))) {
		return nil
	}
	logr.FromContextOrDiscard(ctx).Info("DTK done flag found but no built RPMs, discarding it", "flag", doneFlagPath)
	if err := d.os.RemoveAll(doneFlagPath); err != nil {
		return fmt.Errorf("failed to remove stale done flag: %w", err)
	}
	return nil
}

// errDTKCompileFailed is returned when the sidecar publishes a failure flag instead of a done flag.
var errDTKCompileFailed = errors.New("DTK sidecar reported a failed build")

// dtkBuildMaxAttempts is the first try plus one retry. The retry covers a sidecar that compiled
// a source tree the driver was still replacing; a second empty build is a real failure.
const dtkBuildMaxAttempts = 2

// dtkBuildPollInterval is how often the driver checks the sidecar's flags. Tests shorten it.
var dtkBuildPollInterval = 30 * time.Second

// buildAndWaitDTK prepares sources and waits for the sidecar. A start flag from the current boot
// means a compile is already running, so a restarted driver container waits instead of replacing
// that tree. A failure of the resumed build is prepared once more. A flag from another boot is
// stale and is always replaced.
func (d *driverMgr) buildAndWaitDTK(ctx context.Context, sharedDir, startFlagPath, doneFlagPath, kernelVersion string) error {
	log := logr.FromContextOrDiscard(ctx)
	var waitErr error
	for attempt := 1; attempt <= dtkBuildMaxAttempts; attempt++ {
		resume, err := d.dtkBuildInProgress(startFlagPath)
		if err != nil {
			return err
		}
		if attempt == 1 && resume {
			log.Info("DTK build already in progress for this boot, waiting", "flag", startFlagPath)
		} else if err := d.dtkSetupDriverBuild(ctx, sharedDir, startFlagPath, doneFlagPath, kernelVersion); err != nil {
			return fmt.Errorf("failed to setup DTK build: %w", err)
		}
		waitErr = d.dtkWaitForBuild(ctx, doneFlagPath, startFlagPath)
		if waitErr == nil {
			return nil
		}
		if !errors.Is(waitErr, errDTKCompileFailed) || attempt == dtkBuildMaxAttempts {
			return fmt.Errorf("failed waiting for DTK build: %w", waitErr)
		}
		log.Info("DTK sidecar reported a failed build, preparing sources again", "attempt", attempt, "error", waitErr)
	}
	return fmt.Errorf("failed waiting for DTK build: %w", waitErr)
}

// dtkBuildInProgress reports whether the sidecar was asked to compile during this boot.
// The driver container then waits for that attempt instead of replacing its source tree.
func (d *driverMgr) dtkBuildInProgress(startFlagPath string) (bool, error) {
	bootID, err := dtk.BootIDFrom(d.os.ReadFile)
	if err != nil {
		return false, err
	}
	writtenForBoot, err := dtk.StartFlagWrittenForBoot(d.os.ReadFile, startFlagPath, bootID)
	if err != nil {
		return false, fmt.Errorf("failed to read start flag: %w", err)
	}
	return writtenForBoot, nil
}

// writeStartFlag signals the sidecar to compile. The flag is stamped with the boot ID and
// published atomically so the sidecar never observes an empty file. A failure flag from an
// earlier attempt is cleared first, otherwise the next wait would treat the new attempt as
// already failed.
func (d *driverMgr) writeStartFlag(ctx context.Context, startFlagPath string) error {
	logr.FromContextOrDiscard(ctx).Info("Creating start compile flag", "path", startFlagPath)
	failFlag := filepath.Join(filepath.Dir(startFlagPath), constants.DtkFailedCompileFlag)
	if err := d.os.RemoveAll(failFlag); err != nil {
		return fmt.Errorf("failed to clear compile failure flag: %w", err)
	}
	bootID, err := dtk.BootIDFrom(d.os.ReadFile)
	if err != nil {
		return err
	}
	if err := dtk.PublishFlag(startFlagPath, []byte(bootID)); err != nil {
		return fmt.Errorf("failed to create start flag: %w", err)
	}
	return nil
}

func (d *driverMgr) getDTKAppendDriverBuildFlags(ctx context.Context, kernelVersion string) ([]string, error) {
	appendFlags := d.getAppendDriverBuildFlags(constants.OSTypeRedHat)
	appendFlags = append(appendFlags, "--kernel", kernelVersion)

	versionInfo, err := d.host.GetRedHatVersionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get RedHat version info for DTK driver build: %w", err)
	}

	distroVersion := redHatDistroVersion(versionInfo, kernelVersion)
	if distroVersion == "" {
		return nil, fmt.Errorf("failed to determine RHEL distro version for DTK driver build")
	}

	appendFlags = append(appendFlags, "--distro", "rhel"+distroVersion)
	return appendFlags, nil
}

func redHatDistroVersion(versionInfo *hostutils.RedhatVersionInfo, kernelVersion string) string {
	if versionInfo != nil {
		if versionInfo.RHELVersion != "" {
			return versionInfo.RHELVersion
		}
		if versionInfo.OpenShiftVersion == "" && versionInfo.FullVersion != "" {
			return versionInfo.FullVersion
		}
	}

	if matches := regexp.MustCompile(`\.el([0-9]+)_([0-9]+)`).FindStringSubmatch(kernelVersion); len(matches) == 3 {
		return matches[1] + "." + matches[2]
	}
	if matches := regexp.MustCompile(`\.el([0-9]+)`).FindStringSubmatch(kernelVersion); len(matches) == 2 {
		return matches[1]
	}
	return ""
}

// dtkWaitForBuild waits for the DTK build to complete. A failure flag from the sidecar ends the
// wait immediately so the driver can prepare another attempt.
func (d *driverMgr) dtkWaitForBuild(ctx context.Context, doneFlagPath, startFlagPath string) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Waiting for DTK build to complete", "doneFlag", doneFlagPath)
	failFlagPath := filepath.Join(filepath.Dir(startFlagPath), constants.DtkFailedCompileFlag)

	// Poll every 30s. Air-gapped builds from local RPM repos typically finish in ~180s,
	// but full first-time DTK compilations on slower nodes can exceed 15 min.
	// Max total timeout: sleepSec * totalRetries = 30 * 80 = 2400s (40 min).
	sleepSec := 30
	totalRetries := 80
	totalSleepSec := 0

	for totalRetries > 0 {
		if _, err := d.os.Stat(doneFlagPath); err == nil {
			log.Info("DTK build completed")
			return nil
		}
		if reason, err := d.os.ReadFile(failFlagPath); err == nil {
			detail := strings.TrimSpace(string(reason))
			if detail == "" {
				detail = "no details"
			}
			return fmt.Errorf("%w: %s", errDTKCompileFailed, detail)
		}

		log.Info("Awaiting DTK compilation", "next_query_sec", sleepSec)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(dtkBuildPollInterval):
		}

		totalSleepSec += sleepSec
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
