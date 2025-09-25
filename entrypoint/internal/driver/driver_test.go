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
	"github.com/stretchr/testify/mock"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
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
		tempDir  string
		cfg      config.Config
	)

	BeforeEach(func() {
		cmdMock = cmdMockPkg.NewInterface(GinkgoT())
		hostMock = hostMockPkg.NewInterface(GinkgoT())
		osMock = wrappersMockPkg.NewOSWrapper(GinkgoT())
		ctx = context.Background()
		tempDir = GinkgoT().TempDir()

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

	Context("PreStart", func() {
		Context("when container mode is sources", func() {
			BeforeEach(func() {
				dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
			})

			It("should succeed when all required fields are set", func() {
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "update").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "gcc-11").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/usr/bin/gcc-11", "200").Return("", "", nil)

				err := dm.PreStart(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fail when NVIDIA_NIC_DRIVER_PATH is not set", func() {
				cfg.NvidiaNicDriverPath = ""
				dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

				err := dm.PreStart(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("NVIDIA_NIC_DRIVER_PATH environment variable must be set"))
			})

			It("should validate driver inventory path when set", func() {
				inventoryDir := filepath.Join(tempDir, "inventory")
				Expect(os.MkdirAll(inventoryDir, 0755)).To(Succeed())
				cfg.NvidiaNicDriversInventoryPath = inventoryDir
				dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "update").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "gcc-11").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/usr/bin/gcc-11", "200").Return("", "", nil)

				err := dm.PreStart(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fail when driver inventory path is not a directory", func() {
				inventoryFile := filepath.Join(tempDir, "inventory")
				Expect(os.WriteFile(inventoryFile, []byte("test"), 0644)).To(Succeed())
				cfg.NvidiaNicDriversInventoryPath = inventoryFile
				dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "update").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "gcc-11").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/usr/bin/gcc-11", "200").Return("", "", nil)

				err := dm.PreStart(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("NVIDIA_NIC_DRIVERS_INVENTORY_PATH is not a dir"))
			})

			It("should fail when driver inventory path is not accessible", func() {
				cfg.NvidiaNicDriversInventoryPath = "/nonexistent/path"
				dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				osMock.EXPECT().ReadFile("/proc/version").Return([]byte("Linux version 5.4.0-74-generic (buildd@lcy01-amd64-001) (gcc version 11.5.0) #83-Ubuntu SMP Sat May 8 02:35:39 UTC 2021"), nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "update").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "gcc-11").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "update-alternatives", "--install", "/usr/bin/gcc", "gcc", "/usr/bin/gcc-11", "200").Return("", "", nil)

				err := dm.PreStart(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no such file or directory"))
			})
		})

		Context("when container mode is precompiled", func() {
			BeforeEach(func() {
				dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)
			})

			It("should succeed without additional validation", func() {
				err := dm.PreStart(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when container mode is unknown", func() {
			BeforeEach(func() {
				dm = New("unknown", cfg, cmdMock, hostMock, osMock).(*driverMgr)
			})

			It("should return an error", func() {
				err := dm.PreStart(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown containerMode"))
			})
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

	Context("installUbuntuPrerequisites", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})
		It("should install prerequisites for standard kernel", func() {
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-generic").Return("", "", nil)

			err := dm.installUbuntuPrerequisites(ctx, "5.4.0-42-generic")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should copy APT configuration for RT kernel", func() {
			cmdMock.EXPECT().RunCommand(ctx, "cp", "-r", "/host/etc/apt/*", "/etc/apt/").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-realtime").Return("", "", nil)

			err := dm.installUbuntuPrerequisites(ctx, "5.4.0-42-realtime")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when APT update fails", func() {
			expectedError := errors.New("apt update failed")
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", expectedError)

			err := dm.installUbuntuPrerequisites(ctx, "5.4.0-42-generic")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update apt packages"))
		})

		It("should return error when package installation fails", func() {
			expectedError := errors.New("package install failed")
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-generic").Return("", "", expectedError)

			err := dm.installUbuntuPrerequisites(ctx, "5.4.0-42-generic")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install Ubuntu prerequisites"))
		})

		It("should return error when APT configuration copy fails for RT kernel", func() {
			expectedError := errors.New("copy failed")
			cmdMock.EXPECT().RunCommand(ctx, "cp", "-r", "/host/etc/apt/*", "/etc/apt/").Return("", "", expectedError)

			err := dm.installUbuntuPrerequisites(ctx, "5.4.0-42-realtime")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to copy APT configuration from host"))
		})
	})

	Context("installSLESPrerequisites", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should install prerequisites for standard SLES kernel", func() {
			cmdMock.EXPECT().RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "kernel-default-devel=5.4.0-42").Return("", "", nil)

			err := dm.installSLESPrerequisites(ctx, "5.4.0-42-default")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should install prerequisites for kernel without -default suffix", func() {
			cmdMock.EXPECT().RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "kernel-default-devel=5.4.0-42").Return("", "", nil)

			err := dm.installSLESPrerequisites(ctx, "5.4.0-42")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when zypper install fails", func() {
			expectedError := errors.New("zypper install failed")
			cmdMock.EXPECT().RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "kernel-default-devel=5.4.0-42").Return("", "", expectedError)

			err := dm.installSLESPrerequisites(ctx, "5.4.0-42-default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install SLES prerequisites"))
		})

		It("should handle complex kernel version with multiple dashes", func() {
			cmdMock.EXPECT().RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "kernel-default-devel=5.4.0-42.1-1").Return("", "", nil)

			err := dm.installSLESPrerequisites(ctx, "5.4.0-42.1-1-default")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle kernel version with no -default suffix", func() {
			cmdMock.EXPECT().RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "kernel-default-devel=5.4.0-42").Return("", "", nil)

			err := dm.installSLESPrerequisites(ctx, "5.4.0-42")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("getArchitecture", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should return architecture from uname -m", func() {
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			arch := dm.getArchitecture(ctx)
			Expect(arch).To(Equal("x86_64"))
		})

		It("should return x86_64 fallback when uname fails", func() {
			expectedError := errors.New("uname failed")
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("", "", expectedError)

			arch := dm.getArchitecture(ctx)
			Expect(arch).To(Equal("x86_64"))
		})

		It("should trim whitespace from uname output", func() {
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("  aarch64  ", "", nil)

			arch := dm.getArchitecture(ctx)
			Expect(arch).To(Equal("aarch64"))
		})

		It("should handle different architectures", func() {
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("arm64", "", nil)

			arch := dm.getArchitecture(ctx)
			Expect(arch).To(Equal("arm64"))
		})
	})

	Context("getPackageSuffix", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should return -modules for Ubuntu", func() {
			suffix := dm.getPackageSuffix(constants.OSTypeUbuntu)
			Expect(suffix).To(Equal("-modules"))
		})

		It("should return empty string for SLES", func() {
			suffix := dm.getPackageSuffix(constants.OSTypeSLES)
			Expect(suffix).To(Equal(""))
		})

		It("should return empty string for RedHat", func() {
			suffix := dm.getPackageSuffix(constants.OSTypeRedHat)
			Expect(suffix).To(Equal(""))
		})

		It("should return empty string for OpenShift", func() {
			suffix := dm.getPackageSuffix(constants.OSTypeOpenShift)
			Expect(suffix).To(Equal(""))
		})

		It("should return empty string for unknown OS", func() {
			suffix := dm.getPackageSuffix("unknown")
			Expect(suffix).To(Equal(""))
		})
	})

	Context("getAppendDriverBuildFlags", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should return additional flags when EnableNfsRdma is false for Ubuntu", func() {
			cfg.EnableNfsRdma = false
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			flags := dm.getAppendDriverBuildFlags(constants.OSTypeUbuntu)
			Expect(flags).To(Equal([]string{
				"--without-mlnx-nfsrdma-modules",
				"--without-mlnx-nvme-modules",
			}))
		})

		It("should return additional flags when EnableNfsRdma is false for SLES", func() {
			cfg.EnableNfsRdma = false
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			flags := dm.getAppendDriverBuildFlags(constants.OSTypeSLES)
			Expect(flags).To(Equal([]string{
				"--without-mlnx-nfsrdma",
				"--without-mlnx-nvme",
			}))
		})

		It("should return additional flags when EnableNfsRdma is false for RedHat", func() {
			cfg.EnableNfsRdma = false
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			flags := dm.getAppendDriverBuildFlags(constants.OSTypeRedHat)
			Expect(flags).To(Equal([]string{
				"--without-mlnx-nfsrdma",
				"--without-mlnx-nvme",
			}))
		})

		It("should return empty flags when EnableNfsRdma is true", func() {
			cfg.EnableNfsRdma = true
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			flags := dm.getAppendDriverBuildFlags(constants.OSTypeUbuntu)
			Expect(flags).To(BeEmpty())
		})
	})

	Context("installRedHatPrerequisites", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should install prerequisites for standard RedHat kernel", func() {
			// Mock GetRedHatVersionInfo
			versionInfo := &host.RedhatVersionInfo{
				MajorVersion:     8,
				FullVersion:      "8.4",
				OpenShiftVersion: "",
			}
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(versionInfo, nil)

			// Mock getArchitecture call for EUS setup
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupEUSRepositories - EUS is available for 8.4
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhel-8-for-x86_64-baseos-eus-rpms").Return("", "", nil)

			// Mock getArchitecture call for kernel packages
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock installKernelPackages - packages are installed one by one
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-headers-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-core-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "--allowerasing").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "kernel-modules-5.4.0-42").Return("", "", nil)

			// Mock installRedHatDependencies
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "elfutils-libelf-devel", "kernel-rpm-macros", "numactl-libs", "lsof", "rpm-build", "patch", "hostname").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "makecache", "--releasever=8.4").Return("", "", nil)

			err := dm.installRedHatPrerequisites(ctx, "5.4.0-42")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should install prerequisites for OpenShift with RHOCP repos", func() {
			// Mock GetRedHatVersionInfo for OpenShift
			versionInfo := &host.RedhatVersionInfo{
				MajorVersion:     8,
				FullVersion:      "8.4",
				OpenShiftVersion: "4.9",
			}
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(versionInfo, nil)

			// Mock getArchitecture call for OpenShift setup
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupOpenShiftRepositories
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhocp-4.9-for-rhel-8-x86_64-rpms").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "makecache", "--releasever=8.4").Return("", "", nil)

			// Mock getArchitecture call for EUS setup
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupEUSRepositories - EUS is available for 8.4
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhel-8-for-x86_64-baseos-eus-rpms").Return("", "", nil)

			// Mock getArchitecture call for kernel packages
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock installKernelPackages - packages are installed one by one
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-headers-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-core-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "--allowerasing").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "kernel-modules-5.4.0-42").Return("", "", nil)

			// Mock installRedHatDependencies
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "elfutils-libelf-devel", "kernel-rpm-macros", "numactl-libs", "lsof", "rpm-build", "patch", "hostname").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "makecache", "--releasever=8.4").Return("", "", nil)

			err := dm.installRedHatPrerequisites(ctx, "5.4.0-42")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should install prerequisites for RT kernel", func() {
			// Mock GetRedHatVersionInfo
			versionInfo := &host.RedhatVersionInfo{
				MajorVersion:     8,
				FullVersion:      "8.4",
				OpenShiftVersion: "",
			}
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(versionInfo, nil)

			// Mock getArchitecture call for EUS setup
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupEUSRepositories - EUS is available for 8.4
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhel-8-for-x86_64-baseos-eus-rpms").Return("", "", nil)

			// Mock getArchitecture call for kernel packages
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupSpecialKernelRepos for RT kernel
			cmdMock.EXPECT().RunCommand(ctx, "cp", "/host/etc/yum.repos.d/redhat.repo", "/etc/yum.repos.d/").Return("", "", nil)

			// Mock installKernelPackages for RT kernel
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "install", "kernel-rt-devel-5.4.0-42.rt7.313.x86_64", "kernel-rt-modules-5.4.0-42.rt7.313.x86_64").Return("", "", nil)

			// Mock installRedHatDependencies
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "elfutils-libelf-devel", "kernel-rpm-macros", "numactl-libs", "lsof", "rpm-build", "patch", "hostname").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "makecache", "--releasever=8.4").Return("", "", nil)

			err := dm.installRedHatPrerequisites(ctx, "5.4.0-42.rt7.313.x86_64")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should install prerequisites for 64k kernel", func() {
			// Mock GetRedHatVersionInfo
			versionInfo := &host.RedhatVersionInfo{
				MajorVersion:     8,
				FullVersion:      "8.4",
				OpenShiftVersion: "",
			}
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(versionInfo, nil)

			// Mock getArchitecture call for EUS setup
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupEUSRepositories - EUS is available for 8.4
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhel-8-for-x86_64-baseos-eus-rpms").Return("", "", nil)

			// Mock getArchitecture call for kernel packages
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupSpecialKernelRepos for 64k kernel
			cmdMock.EXPECT().RunCommand(ctx, "cp", "/host/etc/yum.repos.d/redhat.repo", "/etc/yum.repos.d/").Return("", "", nil)

			// Mock installKernelPackages for 64k kernel
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "install", "kernel-64k-devel-5.4.0-42.64k.x86_64", "kernel-64k-modules-5.4.0-42.64k.x86_64").Return("", "", nil)

			// Mock installRedHatDependencies
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "elfutils-libelf-devel", "kernel-rpm-macros", "numactl-libs", "lsof", "rpm-build", "patch", "hostname").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "makecache", "--releasever=8.4").Return("", "", nil)

			err := dm.installRedHatPrerequisites(ctx, "5.4.0-42.64k.x86_64")
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when GetRedHatVersionInfo fails", func() {
			expectedError := errors.New("failed to get version info")
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(nil, expectedError)

			err := dm.installRedHatPrerequisites(ctx, "5.4.0-42")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get RedHat version info"))
		})

		It("should return error when kernel packages installation fails", func() {
			// Mock GetRedHatVersionInfo
			versionInfo := &host.RedhatVersionInfo{
				MajorVersion:     8,
				FullVersion:      "8.4",
				OpenShiftVersion: "",
			}
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(versionInfo, nil)

			// Mock getArchitecture call for EUS setup
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupEUSRepositories - EUS is available for 8.4
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhel-8-for-x86_64-baseos-eus-rpms").Return("", "", nil)

			// Mock getArchitecture call for kernel packages
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock installKernelPackages failure - first package fails
			expectedError := errors.New("kernel install failed")
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-5.4.0-42").Return("", "", expectedError)

			err := dm.installRedHatPrerequisites(ctx, "5.4.0-42")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install kernel packages"))
		})

		It("should return error when dependencies installation fails", func() {
			// Mock GetRedHatVersionInfo
			versionInfo := &host.RedhatVersionInfo{
				MajorVersion:     8,
				FullVersion:      "8.4",
				OpenShiftVersion: "",
			}
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(versionInfo, nil)

			// Mock getArchitecture call for EUS setup
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock setupEUSRepositories - EUS is available for 8.4
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhel-8-for-x86_64-baseos-eus-rpms").Return("", "", nil)

			// Mock getArchitecture call for kernel packages
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)

			// Mock installKernelPackages success - packages are installed one by one
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-headers-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-core-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "--allowerasing").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "kernel-modules-5.4.0-42").Return("", "", nil)

			// Mock installRedHatDependencies failure
			expectedError := errors.New("dependencies install failed")
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "elfutils-libelf-devel", "kernel-rpm-macros", "numactl-libs", "lsof", "rpm-build", "patch", "hostname").Return("", "", expectedError)

			err := dm.installRedHatPrerequisites(ctx, "5.4.0-42")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install RedHat dependencies"))
		})
	})

	Context("Build", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should skip build for non-sources container mode", func() {
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			err := dm.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when GetKernelVersion fails", func() {
			expectedError := errors.New("failed to get kernel version")
			hostMock.EXPECT().GetKernelVersion(ctx).Return("", expectedError)

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get kernel version"))
		})

		It("should return error when GetOSType fails", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			expectedError := errors.New("failed to get OS type")
			hostMock.EXPECT().GetOSType(ctx).Return("", expectedError)

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get OS type"))
		})

		It("should return error when checkDriverInventory fails", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Set inventory path to trigger the error path
			dm.cfg.NvidiaNicDriversInventoryPath = "/test/inventory"
			osMock.EXPECT().Stat("/test/inventory/5.4.0-42-generic/test-version").Return(nil, errors.New("stat error"))

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check inventory directory"))
		})

		It("should skip build when inventory exists and checksums match", func() {
			// Set up inventory path
			inventoryDir := filepath.Join(tempDir, "inventory")
			Expect(os.MkdirAll(inventoryDir, 0755)).To(Succeed())
			cfg.NvidiaNicDriversInventoryPath = inventoryDir
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return false (skip build) - checksums match
			osMock.EXPECT().Stat(filepath.Join(inventoryDir, "5.4.0-42-generic", "test-version")).Return(nil, nil)          // inventory directory exists
			osMock.EXPECT().Stat(filepath.Join(inventoryDir, "5.4.0-42-generic", "test-version.checksum")).Return(nil, nil) // checksum file exists
			osMock.EXPECT().ReadFile(mock.Anything).Return([]byte("abc123def456"), nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("abc123def456", "", nil)

			// Mock installDriver calls (now always called even when skipping build)
			// Mock kernel modules directory creation
			osMock.EXPECT().Stat("/lib/modules/5.4.0-42-generic").Return(nil, os.ErrNotExist)
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", "/lib/modules/5.4.0-42-generic").Return("", "", nil)

			// Mock touch commands for modules.order and modules.builtin
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-generic/modules.order").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-generic/modules.builtin").Return("", "", nil)

			// Mock installUbuntuDriver calls
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "apt-cache show") && strings.Contains(cmd, "linux-modules-extra-5.4.0-42-generic")
			})).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "apt-get install -y") && strings.Contains(cmd, "*.deb")
			})).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "depmod", "5.4.0-42-generic").Return("", "", nil)

			// Mock ubuntuSyncNetworkConfigurationTools
			osMock.EXPECT().Stat("/etc/network/interfaces").Return(nil, os.ErrNotExist)
			osMock.EXPECT().Stat("/sbin/ifup").Return(nil, nil) // /sbin/ifup exists
			cmdMock.EXPECT().RunCommand(ctx, "mv", "/sbin/ifup", "/sbin/ifup.bk").Return("", "", nil)

			err := dm.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should build driver successfully for Ubuntu", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return true (build needed) - no inventory path set
			// This will cause checkDriverInventory to return true

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installUbuntuPrerequisites
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-generic").Return("", "", nil)

			// Mock buildDriverFromSource - need to mock the actual install.pl command with all arguments
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42-generic", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem-modules", "--without-iser-modules",
				"--without-isert-modules", "--without-srp-modules", "--without-kernel-mft-modules",
				"--without-mlnx-rdma-rxe-modules", "--without-dkms", "--without-mlnx-nfsrdma-modules",
				"--without-mlnx-nvme-modules").Return("", "", nil)

			// Mock copyBuildArtifacts - debug logging and copy
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la source directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // find .deb files
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la destination directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // cp command

			// Note: storeBuildChecksum is not called when NvidiaNicDriversInventoryPath is empty

			// Mock fixSourceLink
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			osMock.EXPECT().Readlink(mock.Anything).Return("", errors.New("not found"))

			// Mock installDriver - check if kernel modules directory exists
			osMock.EXPECT().Stat("/lib/modules/5.4.0-42-generic").Return(nil, os.ErrNotExist)
			// Mock creating kernel modules directory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", "/lib/modules/5.4.0-42-generic").Return("", "", nil)
			// Mock creating modules.order and modules.builtin files
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-generic/modules.order").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-generic/modules.builtin").Return("", "", nil)
			// Mock Ubuntu driver installation
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "apt-get install -y") && strings.Contains(cmd, "*.deb")
			})).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "depmod", "5.4.0-42-generic").Return("", "", nil)

			// Mock ubuntuSyncNetworkConfigurationTools
			osMock.EXPECT().Stat("/etc/network/interfaces").Return(nil, os.ErrNotExist)
			osMock.EXPECT().Stat("/sbin/ifup").Return(nil, os.ErrNotExist)

			err := dm.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should build driver successfully for SLES", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-default", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeSLES, nil)

			// Mock checkDriverInventory to return true (build needed) - no inventory path set
			// This will cause checkDriverInventory to return true

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installSLESPrerequisites
			cmdMock.EXPECT().RunCommand(ctx, "zypper", "--non-interactive", "install", "--no-recommends", "kernel-default-devel=5.4.0-42").Return("", "", nil)

			// Mock buildDriverFromSource - SLES specific arguments
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42-default", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem", "--without-iser",
				"--without-isert", "--without-srp", "--without-kernel-mft",
				"--without-mlnx-rdma-rxe", "--disable-kmp", "--kernel-sources",
				"/lib/modules/5.4.0-42-default/build", "--without-mlnx-nfsrdma",
				"--without-mlnx-nvme").Return("", "", nil)

			// Mock copyBuildArtifacts - debug logging and copy
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la source directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // find .deb files
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la destination directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // cp command

			// Note: storeBuildChecksum is not called when NvidiaNicDriversInventoryPath is empty

			// Mock fixSourceLink
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			osMock.EXPECT().Readlink(mock.Anything).Return("", errors.New("not found"))

			// Mock installDriver - check if kernel modules directory exists
			osMock.EXPECT().Stat("/lib/modules/5.4.0-42-default").Return(nil, os.ErrNotExist)
			// Mock creating kernel modules directory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", "/lib/modules/5.4.0-42-default").Return("", "", nil)
			// Mock creating modules.order and modules.builtin files
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-default/modules.order").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-default/modules.builtin").Return("", "", nil)
			// Mock RedHat driver installation (SLES uses RPM)
			cmdMock.EXPECT().RunCommand(ctx, "rpm", "-ivh", "--replacepkgs", "--nodeps", mock.Anything).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "depmod", "5.4.0-42-default").Return("", "", nil)

			err := dm.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should build driver successfully for RedHat", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)

			// Mock checkDriverInventory to return true (build needed) - no inventory path set
			// This will cause checkDriverInventory to return true

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installRedHatPrerequisites
			versionInfo := &host.RedhatVersionInfo{
				MajorVersion:     8,
				FullVersion:      "8.4",
				OpenShiftVersion: "",
			}
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(versionInfo, nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhel-8-for-x86_64-baseos-eus-rpms").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-headers-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-core-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "--allowerasing").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "kernel-modules-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "elfutils-libelf-devel", "kernel-rpm-macros", "numactl-libs", "lsof", "rpm-build", "patch", "hostname").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "makecache", "--releasever=8.4").Return("", "", nil)

			// Mock buildDriverFromSource - RedHat specific arguments
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem", "--without-iser",
				"--without-isert", "--without-srp", "--without-kernel-mft",
				"--without-mlnx-rdma-rxe", "--disable-kmp", "--without-mlnx-nfsrdma",
				"--without-mlnx-nvme").Return("", "", nil)

			// Mock copyBuildArtifacts - debug logging and copy
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la source directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // find .deb files
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la destination directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // cp command

			// Note: storeBuildChecksum is not called when NvidiaNicDriversInventoryPath is empty

			// Mock fixSourceLink
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			osMock.EXPECT().Readlink(mock.Anything).Return("", errors.New("not found"))

			// Mock installDriver - check if kernel modules directory exists
			osMock.EXPECT().Stat("/lib/modules/5.4.0-42").Return(nil, os.ErrNotExist)
			// Mock creating kernel modules directory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", "/lib/modules/5.4.0-42").Return("", "", nil)
			// Mock creating modules.order and modules.builtin files
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42/modules.order").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42/modules.builtin").Return("", "", nil)
			// Mock RedHat driver installation
			cmdMock.EXPECT().RunCommand(ctx, "rpm", "-ivh", "--replacepkgs", "--nodeps", mock.Anything).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "depmod", "5.4.0-42").Return("", "", nil)

			err := dm.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should build driver successfully for OpenShift", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeOpenShift, nil)

			// Mock checkDriverInventory to return true (build needed) - no inventory path set
			// This will cause checkDriverInventory to return true

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installRedHatPrerequisites for OpenShift
			versionInfo := &host.RedhatVersionInfo{
				MajorVersion:     8,
				FullVersion:      "8.4",
				OpenShiftVersion: "4.9",
			}
			hostMock.EXPECT().GetRedHatVersionInfo(ctx).Return(versionInfo, nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhocp-4.9-for-rhel-8-x86_64-rpms").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "makecache", "--releasever=8.4").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "config-manager", "--set-enabled", "rhel-8-for-x86_64-baseos-eus-rpms").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-headers-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-core-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "--allowerasing").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "kernel-devel-5.4.0-42", "kernel-modules-5.4.0-42").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "dnf", "-q", "-y", "--releasever=8.4", "install", "elfutils-libelf-devel", "kernel-rpm-macros", "numactl-libs", "lsof", "rpm-build", "patch", "hostname").Return("", "", nil)
			// Note: dnf makecache --releasever=8.4 is already called by setupOpenShiftRepositories

			// Mock buildDriverFromSource - OpenShift specific arguments (no --disable-kmp for OpenShift)
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem", "--without-iser",
				"--without-isert", "--without-srp", "--without-kernel-mft",
				"--without-mlnx-rdma-rxe", "--without-mlnx-nfsrdma",
				"--without-mlnx-nvme").Return("", "", nil)

			// Mock copyBuildArtifacts - debug logging and copy
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la source directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // find .deb files
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la destination directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // cp command

			// Note: storeBuildChecksum is not called when NvidiaNicDriversInventoryPath is empty

			// Mock fixSourceLink
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			osMock.EXPECT().Readlink(mock.Anything).Return("", errors.New("not found"))

			// Mock installDriver - check if kernel modules directory exists
			osMock.EXPECT().Stat("/lib/modules/5.4.0-42").Return(nil, os.ErrNotExist)
			// Mock creating kernel modules directory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", "/lib/modules/5.4.0-42").Return("", "", nil)
			// Mock creating modules.order and modules.builtin files
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42/modules.order").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42/modules.builtin").Return("", "", nil)
			// Mock RedHat driver installation (OpenShift uses RPM)
			cmdMock.EXPECT().RunCommand(ctx, "rpm", "-ivh", "--replacepkgs", "--nodeps", mock.Anything).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "depmod", "5.4.0-42").Return("", "", nil)

			err := dm.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when createInventoryDirectory fails", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return true (build needed)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock createInventoryDirectory failure
			expectedError := errors.New("mkdir failed")
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", expectedError)

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create inventory directory"))
		})

		It("should return error when installPrerequisitesForOS fails", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return true (build needed)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installUbuntuPrerequisites failure
			expectedError := errors.New("apt update failed")
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", expectedError)

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install prerequisites"))
		})

		It("should return error when buildDriverFromSource fails", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return true (build needed) - no inventory path set
			// This will cause checkDriverInventory to return true

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installUbuntuPrerequisites
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-generic").Return("", "", nil)

			// Mock buildDriverFromSource failure - Ubuntu specific arguments
			expectedError := errors.New("install.pl failed")
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42-generic", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem-modules", "--without-iser-modules",
				"--without-isert-modules", "--without-srp-modules", "--without-kernel-mft-modules",
				"--without-mlnx-rdma-rxe-modules", "--without-dkms", "--without-mlnx-nfsrdma-modules",
				"--without-mlnx-nvme-modules").Return("", "", expectedError)

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build driver from source"))
		})

		It("should return error when copyBuildArtifacts fails", func() {
			// Set up inventory path
			inventoryDir := filepath.Join(tempDir, "inventory")
			Expect(os.MkdirAll(inventoryDir, 0755)).To(Succeed())
			cfg.NvidiaNicDriversInventoryPath = inventoryDir
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return true (build needed) - inventory directory doesn't exist
			osMock.EXPECT().Stat(mock.Anything).Return(nil, os.ErrNotExist) // inventory directory doesn't exist

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installUbuntuPrerequisites
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-generic").Return("", "", nil)

			// Mock buildDriverFromSource - Ubuntu specific arguments
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42-generic", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem-modules", "--without-iser-modules",
				"--without-isert-modules", "--without-srp-modules", "--without-kernel-mft-modules",
				"--without-mlnx-rdma-rxe-modules", "--without-dkms", "--without-mlnx-nfsrdma-modules",
				"--without-mlnx-nvme-modules").Return("", "", nil)

			// Mock copyBuildArtifacts failure - debug logging and copy failure
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "ls -la") && strings.Contains(cmd, "DEBS")
			})).Return("", "", nil) // ls -la source directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "find") && strings.Contains(cmd, "*.deb")
			})).Return("", "", nil) // find .deb files
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "ls -la") && !strings.Contains(cmd, "DEBS")
			})).Return("", "", nil) // ls -la destination directory
			expectedError := errors.New("cp failed")
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "cp")
			})).Return("", "", expectedError) // cp command fails

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to copy build artifacts"))
		})

		It("should return error when storeBuildChecksum fails", func() {
			// Set up inventory path
			inventoryDir := filepath.Join(tempDir, "inventory")
			Expect(os.MkdirAll(inventoryDir, 0755)).To(Succeed())
			cfg.NvidiaNicDriversInventoryPath = inventoryDir
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return true (build needed) - inventory directory doesn't exist
			osMock.EXPECT().Stat(mock.Anything).Return(nil, os.ErrNotExist) // inventory directory doesn't exist

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installUbuntuPrerequisites
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-generic").Return("", "", nil)

			// Mock buildDriverFromSource - Ubuntu specific arguments
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42-generic", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem-modules", "--without-iser-modules",
				"--without-isert-modules", "--without-srp-modules", "--without-kernel-mft-modules",
				"--without-mlnx-rdma-rxe-modules", "--without-dkms", "--without-mlnx-nfsrdma-modules",
				"--without-mlnx-nvme-modules").Return("", "", nil)

			// Mock copyBuildArtifacts - debug logging and copy
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la source directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // find .deb files
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la destination directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // cp command

			// Mock storeBuildChecksum - return valid checksum
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("abc123def456", "", nil)

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to store build checksum"))
		})

		It("should continue when fixSourceLink fails (non-fatal)", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return true (build needed) - no inventory path set
			// This will cause checkDriverInventory to return true

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installUbuntuPrerequisites
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-generic").Return("", "", nil)

			// Mock buildDriverFromSource - Ubuntu specific arguments
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42-generic", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem-modules", "--without-iser-modules",
				"--without-isert-modules", "--without-srp-modules", "--without-kernel-mft-modules",
				"--without-mlnx-rdma-rxe-modules", "--without-dkms", "--without-mlnx-nfsrdma-modules",
				"--without-mlnx-nvme-modules").Return("", "", nil)

			// Mock copyBuildArtifacts - debug logging and copy
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la source directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // find .deb files
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la destination directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // cp command

			// Note: storeBuildChecksum is not called when NvidiaNicDriversInventoryPath is empty

			// Mock fixSourceLink failure (should not cause build to fail)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			expectedError := errors.New("readlink failed")
			osMock.EXPECT().Readlink(mock.Anything).Return("", expectedError)

			// Mock installDriver - check if kernel modules directory exists
			osMock.EXPECT().Stat("/lib/modules/5.4.0-42-generic").Return(nil, os.ErrNotExist)
			// Mock creating kernel modules directory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", "/lib/modules/5.4.0-42-generic").Return("", "", nil)
			// Mock creating modules.order and modules.builtin files
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-generic/modules.order").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-generic/modules.builtin").Return("", "", nil)
			// Mock Ubuntu driver installation
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "apt-get install -y") && strings.Contains(cmd, "*.deb")
			})).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "depmod", "5.4.0-42-generic").Return("", "", nil)

			// Mock ubuntuSyncNetworkConfigurationTools
			osMock.EXPECT().Stat("/etc/network/interfaces").Return(nil, os.ErrNotExist)
			osMock.EXPECT().Stat("/sbin/ifup").Return(nil, os.ErrNotExist)

			err := dm.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle unsupported OS type in installPrerequisitesForOS", func() {
			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return("unsupported", nil)

			// Mock checkDriverInventory to return true (build needed) - no inventory path set
			// This will cause checkDriverInventory to return true

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			err := dm.Build(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install prerequisites"))
		})

		It("should skip storeBuildChecksum when inventory path is not set", func() {
			// Don't set inventory path
			cfg.NvidiaNicDriversInventoryPath = ""
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			hostMock.EXPECT().GetKernelVersion(ctx).Return("5.4.0-42-generic", nil)
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock checkDriverInventory to return true (build needed) - no inventory path set
			// This will cause checkDriverInventory to return true

			// Mock createInventoryDirectory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", mock.Anything).Return("", "", nil)

			// Mock installUbuntuPrerequisites
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yq", "install", "pkg-config", "linux-headers-5.4.0-42-generic").Return("", "", nil)

			// Mock buildDriverFromSource - Ubuntu specific arguments
			cmdMock.EXPECT().RunCommand(ctx, "/test/driver/path/install.pl",
				"--without-depcheck", "--kernel", "5.4.0-42-generic", "--kernel-only", "--build-only",
				"--with-mlnx-tools", "--without-knem-modules", "--without-iser-modules",
				"--without-isert-modules", "--without-srp-modules", "--without-kernel-mft-modules",
				"--without-mlnx-rdma-rxe-modules", "--without-dkms", "--without-mlnx-nfsrdma-modules",
				"--without-mlnx-nvme-modules").Return("", "", nil)

			// Mock copyBuildArtifacts - debug logging and copy
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la source directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // find .deb files
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // ls -la destination directory
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil) // cp command

			// Mock fixSourceLink
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			osMock.EXPECT().Readlink(mock.Anything).Return("", errors.New("not found"))

			// Mock installDriver - check if kernel modules directory exists
			osMock.EXPECT().Stat("/lib/modules/5.4.0-42-generic").Return(nil, os.ErrNotExist)
			// Mock creating kernel modules directory
			cmdMock.EXPECT().RunCommand(ctx, "mkdir", "-p", "/lib/modules/5.4.0-42-generic").Return("", "", nil)
			// Mock creating modules.order and modules.builtin files
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-generic/modules.order").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "touch", "/lib/modules/5.4.0-42-generic/modules.builtin").Return("", "", nil)
			// Mock Ubuntu driver installation
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "update").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.MatchedBy(func(cmd string) bool {
				return strings.Contains(cmd, "apt-get install -y") && strings.Contains(cmd, "*.deb")
			})).Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "depmod", "5.4.0-42-generic").Return("", "", nil)

			// Mock ubuntuSyncNetworkConfigurationTools
			osMock.EXPECT().Stat("/etc/network/interfaces").Return(nil, os.ErrNotExist)
			osMock.EXPECT().Stat("/sbin/ifup").Return(nil, os.ErrNotExist)

			err := dm.Build(ctx)
			Expect(err).NotTo(HaveOccurred())
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
