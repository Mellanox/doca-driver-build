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
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
)

func TestRunBuild(t *testing.T) {
	log := logr.Discard()
	tempDir := t.TempDir()

	startFlag := filepath.Join(tempDir, "dtk_start_compile")
	doneFlag := filepath.Join(tempDir, "dtk_done_compile")

	cfg := config.Config{
		DtkOcpStartCompileFlag:  startFlag,
		DtkOcpDoneCompileFlag:   doneFlag,
		DtkOcpCompiledDriverVer: "1.0.0",
		DtkOcpNicSharedDir:      tempDir,
	}

	t.Run("should fail if flags are not set", func(t *testing.T) {
		err := RunBuild(context.Background(), log, config.Config{}, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required DTK environment variables not set")
	})

	t.Run("should fail if perl installation fails", func(t *testing.T) {
		cmdMock := cmdMockPkg.NewInterface(t)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "perl").Return("", "", errors.New("dnf failed"))

		err := RunBuild(context.Background(), log, cfg, cmdMock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to install perl")
	})

	t.Run("should fail if build dependencies installation fails", func(t *testing.T) {
		cmdMock := cmdMockPkg.NewInterface(t)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "perl").Return("", "", nil)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "ethtool", "autoconf", "pciutils", "automake", "libtool", "python3-devel").Return("", "", errors.New("dnf failed"))

		err := RunBuild(context.Background(), log, cfg, cmdMock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to install build dependencies")
	})

	t.Run("should wait for start flag and run build", func(t *testing.T) {
		cmdMock := cmdMockPkg.NewInterface(t)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "perl").Return("", "", nil)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "ethtool", "autoconf", "pciutils", "automake", "libtool", "python3-devel").Return("", "", nil)

		// Create start flag after a short delay
		go func() {
			time.Sleep(100 * time.Millisecond)
			f, err := os.Create(startFlag)
			assert.NoError(t, err)
			f.Close()
		}()

		expectedInstallScript := filepath.Join(tempDir, "MLNX_OFED_SRC-1.0.0", "install.pl")
		cmdMock.EXPECT().RunCommand(mock.Anything, expectedInstallScript,
			"--build-only", "--kernel-only", "--without-knem", "--without-iser", "--without-isert",
			"--without-srp", "--with-mlnx-tools", "--with-ofed-scripts", "--copy-ifnames-udev",
			"--disable-kmp", "--without-dkms").Return("", "", nil)

		// Create a context that we can cancel to simulate end of execution
		ctx, cancel := context.WithCancel(context.Background())
		
		// Run in a goroutine so we can cancel it
		errCh := make(chan error)
		go func() {
			errCh <- RunBuild(ctx, log, cfg, cmdMock)
		}()

		// Wait for done flag to be created
		// Increased timeout to account for retryDelay in RunBuild
		assert.Eventually(t, func() bool {
			_, err := os.Stat(doneFlag)
			return err == nil
		}, 5*time.Second, 100*time.Millisecond)

		// Wait for start flag to be removed
		assert.Eventually(t, func() bool {
			_, err := os.Stat(startFlag)
			return os.IsNotExist(err)
		}, 5*time.Second, 100*time.Millisecond)

		// Cancel context to stop the infinite loop
		cancel()
		
		err := <-errCh
		assert.ErrorIs(t, err, context.Canceled)
	})
}
