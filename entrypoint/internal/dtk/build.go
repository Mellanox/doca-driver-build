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

package dtk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/kballard/go-shellquote"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
)

// RunBuild executes the DTK driver build logic
func RunBuild(ctx context.Context, log logr.Logger, cfg config.Config, cmdHelper cmd.Interface) error {
	log.Info("DTK driver build script start")

	if cfg.DtkOcpStartCompileFlag == "" || cfg.DtkOcpDoneCompileFlag == "" ||
		cfg.DtkOcpNicSharedDir == "" || cfg.DtkOcpCompiledDriverVer == "" {
		err := fmt.Errorf("required DTK environment variables not set: %s, %s, %s, %s",
			cfg.DtkOcpStartCompileFlag, cfg.DtkOcpDoneCompileFlag, cfg.DtkOcpNicSharedDir, cfg.DtkOcpCompiledDriverVer)
		log.Error(err, "aborting")
		return err
	}

	// Install dependencies
	// Req. for /install.pl script
	log.Info("Installing perl")
	if _, _, err := cmdHelper.RunCommand(ctx, "dnf", "install", "-y", "perl"); err != nil {
		return fmt.Errorf("failed to install perl: %w", err)
	}

	// Req. for build
	log.Info("Installing build dependencies")
	deps := []string{"ethtool", "autoconf", "pciutils", "automake", "libtool", "python3-devel"}
	args := append([]string{"install", "-y"}, deps...)
	if _, _, err := cmdHelper.RunCommand(ctx, "dnf", args...); err != nil {
		return fmt.Errorf("failed to install build dependencies: %w", err)
	}

	// Wait for start flag
	retryDelay := 3 * time.Second
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if _, err := os.Stat(cfg.DtkOcpStartCompileFlag); err == nil {
			break
		}
		log.Info("Awaiting driver container preparations prior compilation", "next_query_sec", retryDelay.Seconds())

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay):
		}
	}

	log.Info("Starting compilation of driver", "version", cfg.DtkOcpCompiledDriverVer)

	// Construct install.pl command
	// ${DTK_OCP_NIC_SHARED_DIR}/MLNX_OFED_SRC-${DTK_OCP_COMPILED_DRIVER_VER}/install.pl
	srcDirName := fmt.Sprintf("MLNX_OFED_SRC-%s", cfg.DtkOcpCompiledDriverVer)
	installScript := filepath.Join(cfg.DtkOcpNicSharedDir, srcDirName, "install.pl")

	installArgs := []string{
		installScript,
		"--build-only",
		"--kernel-only",
		"--without-knem",
		"--without-iser",
		"--without-isert",
		"--without-srp",
		"--with-mlnx-tools",
		"--with-ofed-scripts",
		"--copy-ifnames-udev",
		"--disable-kmp",
		"--without-dkms",
	}

	if cfg.AppendDriverBuildFlags != "" {
		// Use shell-style parsing to handle quoted arguments correctly
		flags, err := shellquote.Split(cfg.AppendDriverBuildFlags)
		if err != nil {
			return fmt.Errorf("failed to parse APPEND_DRIVER_BUILD_FLAGS: %w", err)
		}
		installArgs = append(installArgs, flags...)
	}

	// Execute build
	log.Info("Executing build command", "command", installArgs[0], "args", installArgs[1:])
	if _, _, err := cmdHelper.RunCommand(ctx, installArgs[0], installArgs[1:]...); err != nil {
		// Check if error is context canceled
		if ctx.Err() != nil {
			log.Info("Build canceled by context")
			return ctx.Err()
		}
		return fmt.Errorf("driver build failed: %w", err)
	}

	// Create done flag
	if _, err := os.Create(cfg.DtkOcpDoneCompileFlag); err != nil {
		return fmt.Errorf("failed to create done flag: %w", err)
	}

	// Remove start flag
	if err := os.Remove(cfg.DtkOcpStartCompileFlag); err != nil {
		log.Error(err, "failed to remove start flag")
		// Non-fatal
	}

	log.Info("DTK driver build script end")

	// Sleep infinity with context support
	log.Info("Build completed, sleeping indefinitely")
	<-ctx.Done()
	return ctx.Err()
}
