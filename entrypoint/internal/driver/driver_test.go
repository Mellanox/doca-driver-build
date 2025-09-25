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
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
	hostMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host/mocks"
	wrappersMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers/mocks"
)

var _ = Describe("Driver", func() {
	var (
		dm       *driverMgr
		cmdMock  *cmdMockPkg.Interface
		hostMock *hostMockPkg.Interface
		osMock   *wrappersMockPkg.OSWrapper
		ctx      context.Context
		cfg      config.Config
	)

	BeforeEach(func() {
		cmdMock = cmdMockPkg.NewInterface(GinkgoT())
		hostMock = hostMockPkg.NewInterface(GinkgoT())
		osMock = wrappersMockPkg.NewOSWrapper(GinkgoT())
		ctx = context.Background()

		cfg = config.Config{
			NvidiaNicDriverVer:    "test-version",
			NvidiaNicDriverPath:   "/test/driver/path",
			NvidiaNicContainerVer: "test-container-version",
		}
	})

	Context("New", func() {
		It("should create a new driver manager instance", func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			Expect(dm).NotTo(BeNil())
			Expect(dm.cfg).To(Equal(cfg))
			Expect(dm.containerMode).To(Equal(constants.DriverContainerModeSources))
			Expect(dm.cmd).To(Equal(cmdMock))
			Expect(dm.host).To(Equal(hostMock))
			Expect(dm.os).To(Equal(osMock))
		})
	})

	Context("prepareGCC", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		Context("when OS type is OpenShift", func() {
			It("should skip GCC setup and return nil", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeOpenShift, nil)

				err := dm.prepareGCC(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when host.GetOSType returns error", func() {
			It("should return error", func() {
				expectedErr := errors.New("failed to get OS type")
				hostMock.EXPECT().GetOSType(ctx).Return("", expectedErr)

				err := dm.prepareGCC(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get OS type"))
			})
		})

		Context("when os.ReadFile fails to read /proc/version", func() {
			It("should return error", func() {
				expectedErr := errors.New("failed to read file")
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return(nil, expectedErr)

				err := dm.prepareGCC(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to read /proc/version"))
			})
		})

		Context("when no GCC version can be extracted from /proc/version", func() {
			It("should return nil without error", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (clang version 9.3.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)

				err := dm.prepareGCC(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when OS type is Ubuntu", func() {
			It("should install gcc-X package and set up alternatives", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)

				// Mock apt-get update
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "update").Return("", "", nil)
				// Mock apt-get install gcc-11
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "gcc-11").Return("", "", nil)
				// Mock update-alternatives
				cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/usr/bin/gcc-11", "200").Return("", "", nil)

				err := dm.prepareGCC(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return error when apt-get update fails", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)

				expectedErr := errors.New("apt-get update failed")
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "update").Return("", "", expectedErr)

				err := dm.prepareGCC(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update apt packages"))
			})

			It("should return error when apt-get install fails", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)

				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "update").Return("", "", nil)
				expectedErr := errors.New("apt-get install failed")
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "gcc-11").Return("", "", expectedErr)

				err := dm.prepareGCC(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install gcc-11"))
			})

			It("should return error when update-alternatives fails", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)

				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "update").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "gcc-11").Return("", "", nil)
				expectedErr := errors.New("update-alternatives failed")
				cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/usr/bin/gcc-11", "200").Return("", "", expectedErr)

				err := dm.prepareGCC(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to set up GCC alternatives"))
			})
		})

		Context("when OS type is SLES", func() {
			It("should install gccX package and set up alternatives", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeSLES, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.3.18-59.27-default (gcc version 9.2.1 20190903) #1 SMP Wed Aug 14 12:54:40 UTC 2019"), nil)

				// Mock zypper install
				cmdMock.EXPECT().RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "gcc9").Return("", "", nil)
				// Mock update-alternatives
				cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/usr/bin/gcc-9", "200").Return("", "", nil)

				err := dm.prepareGCC(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return error when zypper install fails", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeSLES, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.3.18-59.27-default (gcc version 9.2.1 20190903) #1 SMP Wed Aug 14 12:54:40 UTC 2019"), nil)

				expectedErr := errors.New("zypper install failed")
				cmdMock.EXPECT().RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "gcc9").Return("", "", expectedErr)

				err := dm.prepareGCC(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install gcc9"))
			})
		})

		Context("when OS type is RedHat", func() {
			Context("when gcc-toolset is available", func() {
				It("should install gcc-toolset and set up alternatives", func() {
					hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)
					osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 4.18.0-477.13.1.el8_8.x86_64 (mockbuild@kbuilder.bsys.centos.org) (gcc version 8.5.0 20210514) #1 SMP Wed Oct 11 14:12:32 UTC 2023"), nil)

					// Mock dnf list available (success - toolset available)
					cmdMock.EXPECT().RunCommand(ctx, "dnf", "list", "available", "gcc-toolset-8").Return("", "", nil)
					// Mock dnf install gcc-toolset
					cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "install", "gcc-toolset-8").Return("", "", nil)
					// Mock update-alternatives
					cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/opt/rh/gcc-toolset-8/root/usr/bin/gcc", "200").Return("", "", nil)

					err := dm.prepareGCC(ctx)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return error when dnf install gcc-toolset fails", func() {
					hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)
					osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 4.18.0-477.13.1.el8_8.x86_64 (mockbuild@kbuilder.bsys.centos.org) (gcc version 8.5.0 20210514) #1 SMP Wed Oct 11 14:12:32 UTC 2023"), nil)

					cmdMock.EXPECT().RunCommand(ctx, "dnf", "list", "available", "gcc-toolset-8").Return("", "", nil)
					expectedErr := errors.New("dnf install failed")
					cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "install", "gcc-toolset-8").Return("", "", expectedErr)

					err := dm.prepareGCC(ctx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to install gcc-toolset-8"))
				})
			})

			Context("when gcc-toolset is not available", func() {
				It("should fall back to default gcc package", func() {
					hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)
					osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 4.18.0-477.13.1.el8_8.x86_64 (mockbuild@kbuilder.bsys.centos.org) (gcc version 8.5.0 20210514) #1 SMP Wed Oct 11 14:12:32 UTC 2023"), nil)

					// Mock dnf list available (failure - toolset not available)
					expectedErr := errors.New("package not found")
					cmdMock.EXPECT().RunCommand(ctx, "dnf", "list", "available", "gcc-toolset-8").Return("", "", expectedErr)
					// Mock dnf install gcc
					cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "install", "gcc").Return("", "", nil)
					// Mock update-alternatives
					cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/usr/bin/gcc", "200").Return("", "", nil)

					err := dm.prepareGCC(ctx)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return error when dnf install gcc fails", func() {
					hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)
					osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 4.18.0-477.13.1.el8_8.x86_64 (mockbuild@kbuilder.bsys.centos.org) (gcc version 8.5.0 20210514) #1 SMP Wed Oct 11 14:12:32 UTC 2023"), nil)

					expectedErr := errors.New("package not found")
					cmdMock.EXPECT().RunCommand(ctx, "dnf", "list", "available", "gcc-toolset-8").Return("", "", expectedErr)
					expectedErr2 := errors.New("dnf install gcc failed")
					cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "install", "gcc").Return("", "", expectedErr2)

					err := dm.prepareGCC(ctx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("failed to install gcc"))
				})
			})
		})

		Context("when OS type is unsupported", func() {
			It("should return error", func() {
				hostMock.EXPECT().GetOSType(ctx).Return("unsupported-os", nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)

				err := dm.prepareGCC(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unsupported OS type: unsupported-os"))
			})
		})
	})
})

var _ = Describe("Driver OFED Blacklist", func() {
	Context("generateOfedModulesBlacklist", func() {
		var (
			dm       *driverMgr
			cmdMock  *cmdMockPkg.Interface
			hostMock *hostMockPkg.Interface
			ctx      context.Context
			tempDir  string
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			ctx = context.Background()
			tempDir = GinkgoT().TempDir()
		})

		It("should create blacklist file with all modules", func() {
			blacklistFile := filepath.Join(tempDir, "blacklist-ofed-modules.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules: []string{
					"mlx5_core",
					"mlx5_ib",
					"ib_umad",
					"ib_uverbs",
					"ib_ipoib",
					"rdma_cm",
					"rdma_ucm",
					"ib_core",
					"ib_cm",
				},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_ib"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_umad"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_uverbs"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_ipoib"))
			Expect(contentStr).To(ContainSubstring("blacklist rdma_cm"))
			Expect(contentStr).To(ContainSubstring("blacklist rdma_ucm"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_core"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_cm"))
		})

		It("should handle empty modules list", func() {
			blacklistFile := filepath.Join(tempDir, "empty-blacklist.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content - should only have header
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))

			// Count blacklist lines - should be 0 (only header comment)
			lines := strings.Split(contentStr, "\n")
			blacklistLines := 0
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "blacklist") {
					blacklistLines++
				}
			}
			Expect(blacklistLines).To(Equal(0))
		})

		It("should skip empty or whitespace-only module names", func() {
			blacklistFile := filepath.Join(tempDir, "filtered-blacklist.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules: []string{
					"mlx5_core",
					"",    // empty string
					"   ", // whitespace only
					"mlx5_ib",
					"\t\n", // whitespace with newlines
					"ib_umad",
				},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_ib"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_umad"))

			// Count blacklist lines - should be 3 (empty and whitespace entries should be skipped)
			lines := strings.Split(contentStr, "\n")
			blacklistLines := 0
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "blacklist") {
					blacklistLines++
				}
			}
			Expect(blacklistLines).To(Equal(3))
		})

		It("should handle single module", func() {
			blacklistFile := filepath.Join(tempDir, "single-module.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))

			// Count blacklist lines - should be 1
			lines := strings.Split(contentStr, "\n")
			blacklistLines := 0
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "blacklist") {
					blacklistLines++
				}
			}
			Expect(blacklistLines).To(Equal(1))
		})
	})

	Context("removeOfedModulesBlacklist", func() {
		var (
			dm       *driverMgr
			cmdMock  *cmdMockPkg.Interface
			hostMock *hostMockPkg.Interface
			ctx      context.Context
			tempDir  string
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			ctx = context.Background()
			tempDir = GinkgoT().TempDir()
		})

		It("should remove existing blacklist file", func() {
			blacklistFile := filepath.Join(tempDir, "blacklist.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// First create the file
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Now remove it
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file is removed
			_, err = os.Stat(blacklistFile)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should handle file not existing gracefully", func() {
			blacklistFile := filepath.Join(tempDir, "nonexistent.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Try to remove non-existent file
			err := dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle multiple remove operations", func() {
			blacklistFile := filepath.Join(tempDir, "multi-remove.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Create file
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Remove it multiple times - should not error
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Integration tests", func() {
		var (
			dm       *driverMgr
			cmdMock  *cmdMockPkg.Interface
			hostMock *hostMockPkg.Interface
			ctx      context.Context
			tempDir  string
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			ctx = context.Background()
			tempDir = GinkgoT().TempDir()
		})

		It("should create and then remove blacklist file", func() {
			blacklistFile := filepath.Join(tempDir, "integration-test.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core", "mlx5_ib", "ib_umad"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Test creation
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists and has correct content
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_ib"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_umad"))

			// Test removal
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file is removed
			_, err = os.Stat(blacklistFile)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should handle complex module list with mixed valid and invalid entries", func() {
			blacklistFile := filepath.Join(tempDir, "complex-test.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules: []string{
					"mlx5_core",
					"", // empty string
					"mlx5_ib",
					"   ", // whitespace only
					"ib_umad",
					"\t\n", // whitespace with newlines
					"ib_uverbs",
					"", // another empty string
					"rdma_cm",
				},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Test creation
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_ib"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_umad"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_uverbs"))
			Expect(contentStr).To(ContainSubstring("blacklist rdma_cm"))

			// Count blacklist lines - should be 5 (empty and whitespace entries should be skipped)
			lines := strings.Split(contentStr, "\n")
			blacklistLines := 0
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "blacklist") {
					blacklistLines++
				}
			}
			Expect(blacklistLines).To(Equal(5))

			// Test removal
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file is removed
			_, err = os.Stat(blacklistFile)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should handle default configuration values", func() {
			blacklistFile := filepath.Join(tempDir, "default-config.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules: []string{
					"mlx5_core",
					"mlx5_ib",
					"ib_umad",
					"ib_uverbs",
					"ib_ipoib",
					"rdma_cm",
					"rdma_ucm",
					"ib_core",
					"ib_cm",
				},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Test creation with default modules
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content matches expected default modules
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))

			// Verify all expected modules are present
			expectedModules := []string{
				"mlx5_core", "mlx5_ib", "ib_umad", "ib_uverbs", "ib_ipoib",
				"rdma_cm", "rdma_ucm", "ib_core", "ib_cm",
			}
			for _, module := range expectedModules {
				Expect(contentStr).To(ContainSubstring("blacklist " + module))
			}

			// Test removal
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file is removed
			_, err = os.Stat(blacklistFile)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})
})
