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
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
)

// useBootID points BootID at a temp file holding id and returns id.
func useBootID(t *testing.T, id string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "boot_id")
	assert.NoError(t, os.WriteFile(path, []byte(id+"\n"), 0o644))
	orig := bootIDPath
	bootIDPath = path
	t.Cleanup(func() { bootIDPath = orig })
	return id
}

// fakeRPM creates RPMS/redhat-release-x/x86_64/a.rpm under srcDir.
func fakeRPM(t *testing.T, srcDir string) {
	t.Helper()
	dir := filepath.Join(srcDir, "RPMS", "redhat-release-9.8", "x86_64")
	assert.NoError(t, os.MkdirAll(dir, 0o755))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "a.rpm"), []byte("x"), 0o644))
}

func TestRunBuild(t *testing.T) {
	log := logr.Discard()
	tempDir := t.TempDir()
	bootID := useBootID(t, "boot-1")

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

	t.Run("should wait for start flag and run build without dkms", func(t *testing.T) {
		cfgNoDKMS := cfg
		cfgNoDKMS.UseDKMS = false
		cfgNoDKMS.AppendDriverBuildFlags = "--kernel 5.14.0-687.13.1.el9_8.x86_64 --distro rhel9.8"

		cmdMock := cmdMockPkg.NewInterface(t)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "perl").Return("", "", nil)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "ethtool", "autoconf", "pciutils", "automake", "libtool", "python3-devel").Return("", "", nil)

		// Create start flag after a short delay
		go func() {
			time.Sleep(100 * time.Millisecond)
			assert.NoError(t, os.WriteFile(startFlag, []byte(bootID), 0o644))
		}()

		expectedInstallScript := filepath.Join(tempDir, "MLNX_OFED_SRC-1.0.0", "install.pl")
		// --without-dkms must be present when UseDKMS is false
		cmdMock.EXPECT().RunCommand(mock.Anything, expectedInstallScript,
			"--build-only", "--kernel-only", "--without-knem", "--without-iser", "--without-isert",
			"--without-srp", "--with-mlnx-tools", "--copy-ifnames-udev",
			"--disable-kmp", "--without-dkms", "--kernel", "5.14.0-687.13.1.el9_8.x86_64",
			"--distro", "rhel9.8").
			Run(func(_ context.Context, _ string, _ ...string) { fakeRPM(t, filepath.Dir(expectedInstallScript)) }).
			Return("", "", nil)

		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error)
		go func() {
			errCh <- RunBuild(ctx, log, cfgNoDKMS, cmdMock)
		}()

		assert.Eventually(t, func() bool {
			_, err := os.Stat(doneFlag)
			return err == nil
		}, 5*time.Second, 100*time.Millisecond)

		assert.Eventually(t, func() bool {
			_, err := os.Stat(startFlag)
			return os.IsNotExist(err)
		}, 5*time.Second, 100*time.Millisecond)

		cancel()
		err := <-errCh
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("should not pass --without-dkms or --disable-kmp when UseDKMS is true", func(t *testing.T) {
		cfgDKMS := cfg
		cfgDKMS.UseDKMS = true

		// Use a fresh set of flag files to avoid state from the previous sub-test.
		startFlagDKMS := filepath.Join(tempDir, "dtk_start_compile_dkms")
		doneFlagDKMS := filepath.Join(tempDir, "dtk_done_compile_dkms")
		cfgDKMS.DtkOcpStartCompileFlag = startFlagDKMS
		cfgDKMS.DtkOcpDoneCompileFlag = doneFlagDKMS

		cmdMock := cmdMockPkg.NewInterface(t)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "perl").Return("", "", nil)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "ethtool", "autoconf", "pciutils", "automake", "libtool", "python3-devel").Return("", "", nil)

		go func() {
			time.Sleep(100 * time.Millisecond)
			assert.NoError(t, os.WriteFile(startFlagDKMS, []byte(bootID), 0o644))
		}()

		expectedInstallScript := filepath.Join(tempDir, "MLNX_OFED_SRC-1.0.0", "install.pl")
		// When UseDKMS=true, neither --without-dkms nor --disable-kmp should be passed.
		// This causes install.pl to produce both a DKMS source package (for dkms add
		// registration in the main container) and pre-compiled kmod binary packages
		// (which place .ko files without needing kernel headers in the main container).
		cmdMock.EXPECT().RunCommand(mock.Anything, expectedInstallScript,
			"--build-only", "--kernel-only", "--without-knem", "--without-iser", "--without-isert",
			"--without-srp", "--with-mlnx-tools", "--copy-ifnames-udev").
			Run(func(_ context.Context, _ string, _ ...string) { fakeRPM(t, filepath.Dir(expectedInstallScript)) }).
			Return("", "", nil)

		ctx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error)
		go func() {
			errCh <- RunBuild(ctx, log, cfgDKMS, cmdMock)
		}()

		assert.Eventually(t, func() bool {
			_, err := os.Stat(doneFlagDKMS)
			return err == nil
		}, 5*time.Second, 100*time.Millisecond)

		assert.Eventually(t, func() bool {
			_, err := os.Stat(startFlagDKMS)
			return os.IsNotExist(err)
		}, 5*time.Second, 100*time.Millisecond)

		cancel()
		err := <-errCh
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("should not write done flag when the build produced no RPMs", func(t *testing.T) {
		cfgEmpty := cfg
		dir := t.TempDir()
		cfgEmpty.DtkOcpNicSharedDir = dir
		cfgEmpty.DtkOcpStartCompileFlag = filepath.Join(dir, "dtk_start_compile")
		cfgEmpty.DtkOcpDoneCompileFlag = filepath.Join(dir, "dtk_done_compile")
		assert.NoError(t, os.WriteFile(cfgEmpty.DtkOcpStartCompileFlag, []byte(bootID), 0o644))

		cmdMock := cmdMockPkg.NewInterface(t)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "perl").Return("", "", nil)
		cmdMock.EXPECT().RunCommand(mock.Anything, "dnf", "install", "-y", "ethtool", "autoconf", "pciutils", "automake", "libtool", "python3-devel").Return("", "", nil)
		cmdMock.EXPECT().RunCommand(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("", "", nil)

		err := RunBuild(context.Background(), log, cfgEmpty, cmdMock)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "produced no RPMs")
		_, statErr := os.Stat(cfgEmpty.DtkOcpDoneCompileFlag)
		assert.True(t, os.IsNotExist(statErr))
		_, statErr = os.Stat(cfgEmpty.DtkOcpStartCompileFlag)
		assert.True(t, os.IsNotExist(statErr))
		failFlag := filepath.Join(dir, constants.DtkFailedCompileFlag)
		failBody, readErr := os.ReadFile(failFlag)
		assert.NoError(t, readErr)
		assert.Contains(t, string(failBody), "produced no RPMs")
	})
}

func TestStartFlagIsCurrent(t *testing.T) {
	log := logr.Discard()
	useBootID(t, "boot-2")
	flag := filepath.Join(t.TempDir(), "dtk_start_compile")

	assert.False(t, startFlagIsCurrent(log, flag), "missing flag")

	assert.NoError(t, os.WriteFile(flag, []byte("boot-1"), 0o644))
	assert.False(t, startFlagIsCurrent(log, flag), "flag from previous boot")
	data, err := os.ReadFile(flag)
	assert.NoError(t, err, "a stale flag must be left for the driver to replace")
	assert.Equal(t, "boot-1", string(data))

	assert.NoError(t, os.WriteFile(flag, []byte(""), 0o644))
	assert.False(t, startFlagIsCurrent(log, flag), "empty flag")
	_, err = os.Stat(flag)
	assert.NoError(t, err, "an empty flag must not be deleted")

	assert.NoError(t, os.WriteFile(flag, []byte("boot-2"), 0o644))
	assert.True(t, startFlagIsCurrent(log, flag), "flag from current boot")
}

func TestPublishFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dtk_start_compile")
	assert.NoError(t, os.WriteFile(path, []byte("old"), 0o644))
	assert.NoError(t, PublishFlag(path, []byte("boot-2")))

	data, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "boot-2", string(data))

	entries, err := os.ReadDir(dir)
	assert.NoError(t, err)
	assert.Len(t, entries, 1, "temporary flag must not be left behind")
}

func TestHasBuiltRPMs(t *testing.T) {
	src := t.TempDir()
	assert.False(t, HasBuiltRPMs(src))
	assert.NoError(t, os.MkdirAll(filepath.Join(src, "RPMS"), 0o755))
	assert.False(t, HasBuiltRPMs(src), "empty RPMS")
	fakeRPM(t, src)
	assert.True(t, HasBuiltRPMs(src))
}
