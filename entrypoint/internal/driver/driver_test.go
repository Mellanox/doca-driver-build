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
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
	hostMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host/mocks"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
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
				// Mock updateCACertificates call
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

				// Mock the main PreStart logic
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

				// Mock updateCACertificates call
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

				err := dm.PreStart(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("NVIDIA_NIC_DRIVER_PATH environment variable must be set"))
			})

			It("should validate driver inventory path when set", func() {
				inventoryDir := filepath.Join(tempDir, "inventory")
				Expect(os.MkdirAll(inventoryDir, 0755)).To(Succeed())
				cfg.NvidiaNicDriversInventoryPath = inventoryDir
				dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

				// Mock updateCACertificates call
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

				// Mock the main PreStart logic
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

				// Mock updateCACertificates call
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

				// Mock the main PreStart logic
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

				// Mock updateCACertificates call
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

				// Mock the main PreStart logic
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
				// Mock updateCACertificates call
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

				err := dm.PreStart(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when container mode is unknown", func() {
			BeforeEach(func() {
				dm = New("unknown", cfg, cmdMock, hostMock, osMock).(*driverMgr)
			})

			It("should return an error", func() {
				// Mock updateCACertificates call
				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)
				cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

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
				"--without-mlnx-rdma-rxe-modules", "--disable-kmp", "--without-dkms", "--without-mlnx-nfsrdma-modules",
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
				"--without-mlnx-rdma-rxe", "--disable-kmp", "--without-dkms", "--kernel-sources",
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
				"--without-mlnx-rdma-rxe", "--disable-kmp", "--without-dkms", "--without-mlnx-nfsrdma",
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
				"--without-mlnx-rdma-rxe-modules", "--disable-kmp", "--without-dkms", "--without-mlnx-nfsrdma-modules",
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
				"--without-mlnx-rdma-rxe-modules", "--disable-kmp", "--without-dkms", "--without-mlnx-nfsrdma-modules",
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
				"--without-mlnx-rdma-rxe-modules", "--disable-kmp", "--without-dkms", "--without-mlnx-nfsrdma-modules",
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
				"--without-mlnx-rdma-rxe-modules", "--disable-kmp", "--without-dkms", "--without-mlnx-nfsrdma-modules",
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
				"--without-mlnx-rdma-rxe-modules", "--disable-kmp", "--without-dkms", "--without-mlnx-nfsrdma-modules",
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

	Context("Load", func() {
		BeforeEach(func() {
			// Create a temporary blacklist file for testing
			blacklistFile := filepath.Join(tempDir, "blacklist-ofed-modules.conf")
			cfg.OfedBlacklistModulesFile = blacklistFile
			cfg.OfedBlacklistModules = []string{"mlx5_core", "mlx5_ib", "ib_core"}

			// Use real OS wrapper for file operations, but mocks for other operations
			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}
		})

		It("should return true when modules match and no restart is needed", func() {
			// Mock checkLoadedKmodSrcverVsModinfo to return true (modules match)
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
				"mlx5_ib":   {Name: "mlx5_ib", RefCount: 1, UsedBy: []string{}},
				"ib_core":   {Name: "ib_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo calls for each module
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_ib").Return("srcversion: DEF456", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_ib/srcversion").Return("DEF456", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "ib_core").Return("srcversion: GHI789", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/ib_core/srcversion").Return("GHI789", "", nil)

			// Mock printLoadedDriverVersion
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)
			cmdMock.EXPECT().RunCommand(ctx, "ls", "/sys/class/net/").Return("eth0 eth1", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth0/device/driver").Return("../../../../bus/pci/drivers/mlx5_core", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "ethtool", "--driver", "eth0").Return("version: 5.0-1.0.0", "", nil)

			// Mock mountRootfs (mount already exists scenario)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return("/usr/src/ on /run/mellanox/drivers/usr/src/ type none", "", nil)

			result, err := dm.Load(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
			Expect(dm.newDriverLoaded).To(BeFalse())
		})

		It("should restart driver when modules don't match", func() {
			// Mock checkLoadedKmodSrcverVsModinfo to return false (modules don't match)
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
				"mlx5_ib":   {Name: "mlx5_ib", RefCount: 1, UsedBy: []string{}},
				"ib_core":   {Name: "ib_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo calls - first module has different srcversion
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("XYZ789", "", nil)

			// Mock restartDriver
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", errors.New("not found"))

			// Mock printLoadedDriverVersion
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)
			cmdMock.EXPECT().RunCommand(ctx, "ls", "/sys/class/net/").Return("eth0 eth1", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth0/device/driver").Return("../../../../bus/pci/drivers/mlx5_core", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "ethtool", "--driver", "eth0").Return("version: 5.0-1.0.0", "", nil)

			// Mock mountRootfs (mount already exists scenario)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return("/usr/src/ on /run/mellanox/drivers/usr/src/ type none", "", nil)

			result, err := dm.Load(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
			Expect(dm.newDriverLoaded).To(BeTrue())
		})

		It("should include NFS RDMA modules when enabled", func() {
			cfg.EnableNfsRdma = true
			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Mock checkLoadedKmodSrcverVsModinfo to return false (modules don't match)
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
				"mlx5_ib":   {Name: "mlx5_ib", RefCount: 1, UsedBy: []string{}},
				"ib_core":   {Name: "ib_core", RefCount: 1, UsedBy: []string{}},
				"nvme_rdma": {Name: "nvme_rdma", RefCount: 1, UsedBy: []string{}},
				"rpcrdma":   {Name: "rpcrdma", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo calls - first module has different srcversion
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("XYZ789", "", nil)

			// Mock restartDriver
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", errors.New("not found"))

			// Mock loadNfsRdma
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "rpcrdma").Return("", "", nil)

			// Mock printLoadedDriverVersion
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)
			cmdMock.EXPECT().RunCommand(ctx, "ls", "/sys/class/net/").Return("eth0 eth1", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth0/device/driver").Return("../../../../bus/pci/drivers/mlx5_core", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "ethtool", "--driver", "eth0").Return("version: 5.0-1.0.0", "", nil)

			// Mock mountRootfs (mount already exists scenario)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return("/usr/src/ on /run/mellanox/drivers/usr/src/ type none", "", nil)

			result, err := dm.Load(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
			Expect(dm.newDriverLoaded).To(BeTrue())
		})

		It("should return error when checkLoadedKmodSrcverVsModinfo fails", func() {
			expectedError := errors.New("failed to get loaded modules")
			hostMock.EXPECT().LsMod(ctx).Return(nil, expectedError)

			result, err := dm.Load(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check module versions"))
			Expect(result).To(BeFalse())
		})

		It("should return error when restartDriver fails", func() {
			// Mock checkLoadedKmodSrcverVsModinfo to return false (modules don't match)
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
				"mlx5_ib":   {Name: "mlx5_ib", RefCount: 1, UsedBy: []string{}},
				"ib_core":   {Name: "ib_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo calls - first module has different srcversion
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("XYZ789", "", nil)

			// Mock restartDriver failure
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)
			expectedError := errors.New("openibd restart failed")
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", expectedError)

			result, err := dm.Load(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to restart driver"))
			Expect(result).To(BeFalse())
		})

		It("should continue when loadNfsRdma fails (non-fatal)", func() {
			cfg.EnableNfsRdma = true
			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Mock checkLoadedKmodSrcverVsModinfo to return false (modules don't match)
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
				"mlx5_ib":   {Name: "mlx5_ib", RefCount: 1, UsedBy: []string{}},
				"ib_core":   {Name: "ib_core", RefCount: 1, UsedBy: []string{}},
				"nvme_rdma": {Name: "nvme_rdma", RefCount: 1, UsedBy: []string{}},
				"rpcrdma":   {Name: "rpcrdma", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo calls - first module has different srcversion
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("XYZ789", "", nil)

			// Mock restartDriver
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", errors.New("not found"))

			// Mock loadNfsRdma failure (should not cause Load to fail)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "rpcrdma").Return("", "", errors.New("rpcrdma load failed"))

			// Mock printLoadedDriverVersion
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)
			cmdMock.EXPECT().RunCommand(ctx, "ls", "/sys/class/net/").Return("eth0 eth1", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth0/device/driver").Return("../../../../bus/pci/drivers/mlx5_core", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "ethtool", "--driver", "eth0").Return("version: 5.0-1.0.0", "", nil)

			// Mock mountRootfs (mount already exists scenario)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return("/usr/src/ on /run/mellanox/drivers/usr/src/ type none", "", nil)

			result, err := dm.Load(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
			Expect(dm.newDriverLoaded).To(BeTrue())
		})

	})

	Context("checkLoadedKmodSrcverVsModinfo", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should return true when all modules match", func() {
			modules := []string{"mlx5_core", "mlx5_ib"}

			// Mock LsMod to return loaded modules
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
				"mlx5_ib":   {Name: "mlx5_ib", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo and sysfs calls for each module
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_ib").Return("srcversion: DEF456", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_ib/srcversion").Return("DEF456", "", nil)

			result, err := dm.checkLoadedKmodSrcverVsModinfo(ctx, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})

		It("should return false when module is not loaded", func() {
			modules := []string{"mlx5_core", "mlx5_ib"}

			// Mock LsMod to return only one module loaded
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo and sysfs calls for the loaded module
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("ABC123", "", nil)

			result, err := dm.checkLoadedKmodSrcverVsModinfo(ctx, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should return false when modinfo fails", func() {
			modules := []string{"mlx5_core"}

			// Mock LsMod to return loaded module
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo failure
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("", "", errors.New("modinfo failed"))

			result, err := dm.checkLoadedKmodSrcverVsModinfo(ctx, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should return false when sysfs read fails", func() {
			modules := []string{"mlx5_core"}

			// Mock LsMod to return loaded module
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo success but sysfs failure
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("", "", errors.New("sysfs read failed"))

			result, err := dm.checkLoadedKmodSrcverVsModinfo(ctx, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should return false when srcversions don't match", func() {
			modules := []string{"mlx5_core"}

			// Mock LsMod to return loaded module
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo and sysfs with different srcversions
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("srcversion: ABC123", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("XYZ789", "", nil)

			result, err := dm.checkLoadedKmodSrcverVsModinfo(ctx, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should return error when LsMod fails", func() {
			modules := []string{"mlx5_core"}

			// Mock LsMod failure
			expectedError := errors.New("lsmod failed")
			hostMock.EXPECT().LsMod(ctx).Return(nil, expectedError)

			result, err := dm.checkLoadedKmodSrcverVsModinfo(ctx, modules)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get loaded modules"))
			Expect(result).To(BeFalse())
		})

		It("should handle modinfo output without srcversion", func() {
			modules := []string{"mlx5_core"}

			// Mock LsMod to return loaded module
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock modinfo output without srcversion line
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_core").Return("filename: /lib/modules/5.4.0-42-generic/kernel/drivers/net/ethernet/mellanox/mlx5/core/mlx5_core.ko", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "cat", "/sys/module/mlx5_core/srcversion").Return("ABC123", "", nil)

			result, err := dm.checkLoadedKmodSrcverVsModinfo(ctx, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse()) // Should return false when srcversion not found
		})
	})

	Context("restartDriver", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should restart driver successfully", func() {
			// Mock all the modprobe commands
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", errors.New("not found"))

			err := dm.restartDriver(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should load macsec when mlx5_ib depends on it", func() {
			// Mock all the modprobe commands
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("macsec", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "macsec").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", errors.New("not found"))

			err := dm.restartDriver(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip pci-hyperv-intf on aarch64", func() {
			// Mock all the modprobe commands
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("aarch64", "", nil)
			// pci-hyperv-intf should not be called for aarch64
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", errors.New("not found"))

			err := dm.restartDriver(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should load mlx5_vdpa when available", func() {
			// Mock all the modprobe commands
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", nil) // Module exists
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "mlx5_vdpa").Return("", "", nil)

			err := dm.restartDriver(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should unload storage modules when enabled", func() {
			cfg.UnloadStorageModules = true
			cfg.StorageModules = []string{"ib_isert", "nvme_rdma"}
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock all the modprobe commands
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)

			// Mock unloadStorageModules - first check if mod_load_funcs exists
			osMock.EXPECT().Stat("/usr/share/mlnx_ofed/mod_load_funcs").Return(nil, errors.New("not found"))
			// Then use /etc/init.d/openibd
			cmdMock.EXPECT().RunCommand(ctx, "sed", "-i", "-e", mock.Anything, "/etc/init.d/openibd").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", mock.Anything).Return("1", "", nil)

			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", errors.New("not found"))

			err := dm.restartDriver(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when openibd restart fails", func() {
			// Mock all the modprobe commands
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", nil)

			// Mock openibd restart failure
			expectedError := errors.New("openibd restart failed")
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", expectedError)

			err := dm.restartDriver(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to restart openibd service"))
		})

		It("should continue when non-critical modprobe commands fail", func() {
			// Mock modprobe failures (should not cause restartDriver to fail)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "tls").Return("", "", errors.New("tls load failed"))
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "psample").Return("", "", errors.New("psample load failed"))
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "-F", "depends", "mlx5_ib").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "uname", "-m").Return("x86_64", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "-d", "/host", "pci-hyperv-intf").Return("", "", errors.New("pci-hyperv-intf load failed"))
			cmdMock.EXPECT().RunCommand(ctx, "/etc/init.d/openibd", "restart").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "modinfo", "mlx5_vdpa").Return("", "", errors.New("not found"))

			err := dm.restartDriver(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("loadNfsRdma", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should load rpcrdma when NFS RDMA is enabled", func() {
			cfg.EnableNfsRdma = true
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "rpcrdma").Return("", "", nil)

			err := dm.loadNfsRdma(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return nil when NFS RDMA is disabled", func() {
			cfg.EnableNfsRdma = false
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			err := dm.loadNfsRdma(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when rpcrdma load fails", func() {
			cfg.EnableNfsRdma = true
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			expectedError := errors.New("rpcrdma load failed")
			cmdMock.EXPECT().RunCommand(ctx, "modprobe", "rpcrdma").Return("", "", expectedError)

			err := dm.loadNfsRdma(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to load rpcrdma module"))
		})
	})

	Context("printLoadedDriverVersion", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should print driver version successfully", func() {
			// Mock LsMod to return mlx5_core loaded
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock getFirstMlxNetdevName
			cmdMock.EXPECT().RunCommand(ctx, "ls", "/sys/class/net/").Return("eth0 eth1", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth0/device/driver").Return("../../../../bus/pci/drivers/mlx5_core", "", nil)

			// Mock ethtool
			cmdMock.EXPECT().RunCommand(ctx, "ethtool", "--driver", "eth0").Return("version: 5.0-1.0.0", "", nil)

			err := dm.printLoadedDriverVersion(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return nil when mlx5_core is not loaded", func() {
			// Mock LsMod to return no mlx5_core
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"other_module": {Name: "other_module", RefCount: 1, UsedBy: []string{}},
			}, nil)

			err := dm.printLoadedDriverVersion(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when LsMod fails", func() {
			expectedError := errors.New("lsmod failed")
			hostMock.EXPECT().LsMod(ctx).Return(nil, expectedError)

			err := dm.printLoadedDriverVersion(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check loaded modules"))
		})

		It("should return nil when no Mellanox device found", func() {
			// Mock LsMod to return mlx5_core loaded
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock getFirstMlxNetdevName to return no Mellanox device
			cmdMock.EXPECT().RunCommand(ctx, "ls", "/sys/class/net/").Return("eth0 eth1", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth0/device/driver").Return("../../../../bus/pci/drivers/other_driver", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth1/device/driver").Return("../../../../bus/pci/drivers/another_driver", "", nil)

			err := dm.printLoadedDriverVersion(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return nil when ethtool fails", func() {
			// Mock LsMod to return mlx5_core loaded
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock getFirstMlxNetdevName
			cmdMock.EXPECT().RunCommand(ctx, "ls", "/sys/class/net/").Return("eth0 eth1", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth0/device/driver").Return("../../../../bus/pci/drivers/mlx5_core", "", nil)

			// Mock ethtool failure
			cmdMock.EXPECT().RunCommand(ctx, "ethtool", "--driver", "eth0").Return("", "", errors.New("ethtool failed"))

			err := dm.printLoadedDriverVersion(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle ethtool output without version line", func() {
			// Mock LsMod to return mlx5_core loaded
			hostMock.EXPECT().LsMod(ctx).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil)

			// Mock getFirstMlxNetdevName
			cmdMock.EXPECT().RunCommand(ctx, "ls", "/sys/class/net/").Return("eth0 eth1", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "readlink", "/sys/class/net/eth0/device/driver").Return("../../../../bus/pci/drivers/mlx5_core", "", nil)

			// Mock ethtool output without version line
			cmdMock.EXPECT().RunCommand(ctx, "ethtool", "--driver", "eth0").Return("driver: mlx5_core\nbus-info: 0000:01:00.0", "", nil)

			err := dm.printLoadedDriverVersion(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("updateCACertificates", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should update CA certificates successfully for Ubuntu", func() {
			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock command existence check
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)

			// Mock CA certificate update command
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update CA certificates successfully for SLES", func() {
			// Mock GetOSType to return SLES
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeSLES, nil)

			// Mock command existence check
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)

			// Mock CA certificate update command
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update CA certificates successfully for RedHat", func() {
			// Mock GetOSType to return RedHat
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)

			// Mock command existence check for update-ca-trust
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-trust").Return("", "", nil)

			// Mock CA certificate update command
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-trust extract || true").Return("", "", nil)

			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update CA certificates successfully for OpenShift", func() {
			// Mock GetOSType to return OpenShift
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeOpenShift, nil)

			// Mock command existence check for update-ca-trust
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-trust").Return("", "", nil)

			// Mock CA certificate update command
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-trust extract || true").Return("", "", nil)

			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip CA certificate update for unsupported OS", func() {
			// Mock GetOSType to return unsupported OS
			hostMock.EXPECT().GetOSType(ctx).Return("unsupported", nil)

			// No command execution should happen
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when GetOSType fails", func() {
			expectedError := errors.New("failed to get OS type")
			hostMock.EXPECT().GetOSType(ctx).Return("", expectedError)

			err := dm.updateCACertificates(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get OS type"))
		})

		It("should handle command not found gracefully for Ubuntu", func() {
			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock command existence check failure
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", errors.New("command not found"))

			// No CA certificate update command should be executed
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle command not found gracefully for RedHat", func() {
			// Mock GetOSType to return RedHat
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)

			// Mock command existence check failure
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-trust").Return("", "", errors.New("command not found"))

			// No CA certificate update command should be executed
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle CA certificate update command failure gracefully for Ubuntu", func() {
			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock command existence check
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)

			// Mock CA certificate update command failure
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", errors.New("update failed"))

			// Should not return error (non-fatal)
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle CA certificate update command failure gracefully for RedHat", func() {
			// Mock GetOSType to return RedHat
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)

			// Mock command existence check
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-trust").Return("", "", nil)

			// Mock CA certificate update command failure
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-trust extract || true").Return("", "", errors.New("update failed"))

			// Should not return error (non-fatal)
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle CA certificate update command failure gracefully for SLES", func() {
			// Mock GetOSType to return SLES
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeSLES, nil)

			// Mock command existence check
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)

			// Mock CA certificate update command failure
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", errors.New("update failed"))

			// Should not return error (non-fatal)
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle CA certificate update command failure gracefully for OpenShift", func() {
			// Mock GetOSType to return OpenShift
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeOpenShift, nil)

			// Mock command existence check
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-trust").Return("", "", nil)

			// Mock CA certificate update command failure
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-trust extract || true").Return("", "", errors.New("update failed"))

			// Should not return error (non-fatal)
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should use correct command for Ubuntu with arguments", func() {
			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock command existence check
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-certificates").Return("", "", nil)

			// Mock CA certificate update command - verify the exact command
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-certificates || true").Return("", "", nil)

			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should use correct command for RedHat with arguments", func() {
			// Mock GetOSType to return RedHat
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)

			// Mock command existence check
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-trust").Return("", "", nil)

			// Mock CA certificate update command - verify the exact command
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-trust extract || true").Return("", "", nil)

			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should extract base command correctly from command with arguments", func() {
			// This test verifies that strings.Fields(command)[0] works correctly
			// for extracting the base command from "update-ca-trust extract"

			// Mock GetOSType to return RedHat
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)

			// Mock command existence check - should check for "update-ca-trust" (base command)
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "command -v update-ca-trust").Return("", "", nil)

			// Mock CA certificate update command - should use full command with arguments
			cmdMock.EXPECT().RunCommand(ctx, "sh", "-c", "update-ca-trust extract || true").Return("", "", nil)

			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle empty OS type gracefully", func() {
			// Mock GetOSType to return empty string
			hostMock.EXPECT().GetOSType(ctx).Return("", nil)

			// No command execution should happen
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle nil OS type gracefully", func() {
			// Mock GetOSType to return empty string (nil would be handled by the interface)
			hostMock.EXPECT().GetOSType(ctx).Return("", nil)

			// No command execution should happen
			err := dm.updateCACertificates(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("extractGCCInfo", func() {
		Context("extractGCCVersion", func() {
			It("should extract GCC version from Ubuntu WSL2 format", func() {
				procVersion := "Linux version 6.6.87.1-microsoft-standard-WSL2 (root@af282157c79e) (gcc (GCC) 11.2.0, GNU ld (GNU Binutils) 2.37) #1 SMP PREEMPT_DYNAMIC Mon Apr 21 17:08:54 UTC 2025"
				version, err := dm.extractGCCVersion(procVersion)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal("11.2.0"))
			})

			It("should extract GCC version from SLES format", func() {
				procVersion := "Linux version 6.4.0-150600.21-default (geeko@buildhost) (gcc (SUSE Linux) 7.5.0, GNU ld (GNU Binutils; SUSE Linux Enterprise 15) 2.41.0.20230908-150100.7.46) #1 SMP PREEMPT_DYNAMIC Thu May 16 11:09:22 UTC 2024 (36c1e09)"
				version, err := dm.extractGCCVersion(procVersion)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal("7.5.0"))
			})

			It("should extract GCC version from RHEL format", func() {
				procVersion := "Linux version 5.14.0-570.12.1.el9_6.x86_64 (mockbuild@x86-64-03.build.eng.rdu2.redhat.com) (gcc (GCC) 11.5.0 20240719 (Red Hat 11.5.0-5), GNU ld version 2.35.2-63.el9) #1 SMP PREEMPT_DYNAMIC Fri Apr 4 10:41:31 EDT 2025"
				version, err := dm.extractGCCVersion(procVersion)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal("11.5.0"))
			})

			It("should extract GCC version from Ubuntu format with x86_64-linux-gnu-gcc", func() {
				procVersion := "Linux version 6.8.0-31-generic (buildd@lcy02-amd64-080) (x86_64-linux-gnu-gcc-13 (Ubuntu 13.2.0-23ubuntu4) 13.2.0, GNU ld (GNU Binutils for Ubuntu) 2.42) #31-Ubuntu SMP PREEMPT_DYNAMIC Sat Apr 20 00:40:06 UTC 2024"
				version, err := dm.extractGCCVersion(procVersion)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal("13.2.0"))
			})

			It("should handle GCC version with different patterns", func() {
				testCases := []struct {
					name     string
					input    string
					expected string
				}{
					{
						name:     "Direct GCC version",
						input:    "Linux version 5.4.0 (gcc 9.3.0)",
						expected: "9.3.0",
					},
					{
						name:     "GCC with dash",
						input:    "Linux version 5.4.0 (gcc-9 9.3.0)",
						expected: "9.3.0",
					},
					{
						name:     "GCC with parentheses",
						input:    "Linux version 5.4.0 (gcc (GCC) 8.4.0)",
						expected: "8.4.0",
					},
				}

				for _, tc := range testCases {
					By(tc.name)
					version, err := dm.extractGCCVersion(tc.input)
					Expect(err).NotTo(HaveOccurred())
					Expect(version).To(Equal(tc.expected))
				}
			})

			It("should return error when no GCC version found", func() {
				procVersion := "Linux version 5.4.0 (no gcc here)"
				_, err := dm.extractGCCVersion(procVersion)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no GCC version found in /proc/version"))
			})

			It("should handle empty input", func() {
				_, err := dm.extractGCCVersion("")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no GCC version found in /proc/version"))
			})
		})

		Context("extractMajorVersion", func() {
			It("should extract major version from full version string", func() {
				testCases := []struct {
					version  string
					expected int
				}{
					{"11.2.0", 11},
					{"7.5.0", 7},
					{"13.2.0", 13},
					{"9.3.0", 9},
					{"8.4.0", 8},
				}

				for _, tc := range testCases {
					major, err := dm.extractMajorVersion(tc.version)
					Expect(err).NotTo(HaveOccurred())
					Expect(major).To(Equal(tc.expected))
				}
			})

			It("should handle single digit major version", func() {
				major, err := dm.extractMajorVersion("5.4.0")
				Expect(err).NotTo(HaveOccurred())
				Expect(major).To(Equal(5))
			})

			It("should return error for invalid version format", func() {
				_, err := dm.extractMajorVersion("invalid")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse major version from invalid"))
			})

			It("should return error for empty version", func() {
				_, err := dm.extractMajorVersion("")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to parse major version from"))
			})
		})

	})

	Context("enableFIPSIfRequired", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
		})

		It("should skip FIPS setup when UBUNTU_PRO_TOKEN is not set", func() {
			// Set empty token in config
			dm.cfg.UbuntuProToken = ""

			// No mocks should be called
			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip FIPS setup when not running on Ubuntu", func() {
			// Set Ubuntu Pro token in config
			dm.cfg.UbuntuProToken = "test-token-12345"

			// Mock GetOSType to return RedHat
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeRedHat, nil)

			// No FIPS commands should be executed
			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip FIPS setup when running on SLES", func() {
			// Set Ubuntu Pro token in config
			dm.cfg.UbuntuProToken = "test-token-12345"

			// Mock GetOSType to return SLES
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeSLES, nil)

			// No FIPS commands should be executed
			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should enable FIPS successfully on Ubuntu", func() {
			// Set Ubuntu Pro token in config
			dm.cfg.UbuntuProToken = "test-token-12345"

			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock update-ca-certificates command
			cmdMock.EXPECT().RunCommand(ctx, "update-ca-certificates").Return("", "", nil)

			// Mock pro attach command
			cmdMock.EXPECT().RunCommand(ctx, "pro", "attach", "--no-auto-enable", "test-token-12345").Return("", "", nil)

			// Mock pro enable command
			cmdMock.EXPECT().RunCommand(ctx, "pro", "enable", "--access-only", "--assume-yes", "fips-updates").Return("", "", nil)

			// Mock apt-get install command
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yqq", "install", "--no-install-recommends", "ubuntu-fips-userspace").Return("", "", nil)

			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when GetOSType fails", func() {
			// Set Ubuntu Pro token in config
			dm.cfg.UbuntuProToken = "test-token-12345"

			expectedError := errors.New("failed to get OS type")
			hostMock.EXPECT().GetOSType(ctx).Return("", expectedError)

			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get OS type"))
		})

		It("should return error when update-ca-certificates fails", func() {
			// Set Ubuntu Pro token in config
			dm.cfg.UbuntuProToken = "test-token-12345"

			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock update-ca-certificates command failure
			expectedError := errors.New("ca certificates update failed")
			cmdMock.EXPECT().RunCommand(ctx, "update-ca-certificates").Return("", "", expectedError)

			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update CA certificates"))
		})

		It("should return error when pro attach fails", func() {
			// Set Ubuntu Pro token in config
			dm.cfg.UbuntuProToken = "test-token-12345"

			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock update-ca-certificates command
			cmdMock.EXPECT().RunCommand(ctx, "update-ca-certificates").Return("", "", nil)

			// Mock pro attach command failure
			expectedError := errors.New("pro attach failed")
			cmdMock.EXPECT().RunCommand(ctx, "pro", "attach", "--no-auto-enable", "test-token-12345").Return("", "", expectedError)

			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to attach Ubuntu Pro subscription"))
		})

		It("should return error when pro enable fips-updates fails", func() {
			// Set Ubuntu Pro token in config
			dm.cfg.UbuntuProToken = "test-token-12345"

			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock update-ca-certificates command
			cmdMock.EXPECT().RunCommand(ctx, "update-ca-certificates").Return("", "", nil)

			// Mock pro attach command
			cmdMock.EXPECT().RunCommand(ctx, "pro", "attach", "--no-auto-enable", "test-token-12345").Return("", "", nil)

			// Mock pro enable command failure
			expectedError := errors.New("pro enable failed")
			cmdMock.EXPECT().RunCommand(ctx, "pro", "enable", "--access-only", "--assume-yes", "fips-updates").Return("", "", expectedError)

			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to enable FIPS updates"))
		})

		It("should return error when apt-get install ubuntu-fips-userspace fails", func() {
			// Set Ubuntu Pro token in config
			dm.cfg.UbuntuProToken = "test-token-12345"

			// Mock GetOSType to return Ubuntu
			hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

			// Mock update-ca-certificates command
			cmdMock.EXPECT().RunCommand(ctx, "update-ca-certificates").Return("", "", nil)

			// Mock pro attach command
			cmdMock.EXPECT().RunCommand(ctx, "pro", "attach", "--no-auto-enable", "test-token-12345").Return("", "", nil)

			// Mock pro enable command
			cmdMock.EXPECT().RunCommand(ctx, "pro", "enable", "--access-only", "--assume-yes", "fips-updates").Return("", "", nil)

			// Mock apt-get install command failure
			expectedError := errors.New("apt-get install failed")
			cmdMock.EXPECT().RunCommand(ctx, "apt-get", "-yqq", "install", "--no-install-recommends", "ubuntu-fips-userspace").Return("", "", expectedError)

			err := dm.enableFIPSIfRequired(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to install ubuntu-fips-userspace"))
		})
	})

	Context("mountRootfs", func() {
		It("should successfully mount when no mount exists", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock mount --make-runbindable /sys
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)

			// Mock mount --make-private /sys
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)

			// Mock mount -l to check if mount exists (returns no mellanox mounts)
			mountOutput := "/dev/sda1 on / type ext4 (rw,relatime)\n/dev/sdb1 on /data type ext4 (rw,relatime)\n"
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return(mountOutput, "", nil)

			// Mock mkdir -p for mount path
			osMock.EXPECT().MkdirAll("/run/mellanox/drivers/usr/src", os.FileMode(0o755)).Return(nil)

			// Mock mount --rbind
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--rbind", "/usr/src/", "/run/mellanox/drivers/usr/src").Return("", "", nil)

			err := dm.mountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip mount when mellanox mount already exists", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock mount --make-runbindable /sys
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)

			// Mock mount --make-private /sys
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)

			// Mock mount -l to check if mount exists (returns existing mellanox mount)
			mountOutput := "/dev/sda1 on / type ext4 (rw,relatime)\n/usr/src/ on /run/mellanox/drivers/usr/src/ type none (rw,relatime)\n"
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return(mountOutput, "", nil)

			// Should not call mkdir or mount --rbind when mount exists

			err := dm.mountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip mount when mellanox tmpfs mount exists but not regular mount", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock mount --make-runbindable /sys
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)

			// Mock mount --make-private /sys
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)

			// Mock mount -l to check if mount exists (returns tmpfs mount - should be ignored)
			mountOutput := "/dev/sda1 on / type ext4 (rw,relatime)\ntmpfs on /run/mellanox/tmp type tmpfs (rw,nosuid,nodev,mode=755)\n"
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return(mountOutput, "", nil)

			// Should call mkdir and mount --rbind since tmpfs doesn't count
			osMock.EXPECT().MkdirAll("/run/mellanox/drivers/usr/src", os.FileMode(0o755)).Return(nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--rbind", "/usr/src/", "/run/mellanox/drivers/usr/src").Return("", "", nil)

			err := dm.mountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when mount --make-runbindable fails", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "permission denied", errors.New("mount failed"))

			err := dm.mountRootfs(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to make /sys runbindable"))
		})

		It("should fail when mount --make-private fails", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "permission denied", errors.New("mount failed"))

			err := dm.mountRootfs(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to make /sys private"))
		})

		It("should fail when mkdir fails", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return("", "", nil)
			osMock.EXPECT().MkdirAll("/run/mellanox/drivers/usr/src", os.FileMode(0o755)).Return(errors.New("permission denied"))

			err := dm.mountRootfs(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to create mount directory"))
		})

		It("should fail when mount --rbind fails", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return("", "", nil)
			osMock.EXPECT().MkdirAll("/run/mellanox/drivers/usr/src", os.FileMode(0o755)).Return(nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--rbind", "/usr/src/", "/run/mellanox/drivers/usr/src").Return("", "mount failed", errors.New("mount error"))

			err := dm.mountRootfs(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to rbind mount"))
		})

		It("should handle mount -l failure gracefully and proceed with mount", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-runbindable", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--make-private", "/sys").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "-l").Return("", "", errors.New("mount command failed"))

			// Should proceed with mounting even if mount -l fails
			osMock.EXPECT().MkdirAll("/run/mellanox/drivers/usr/src", os.FileMode(0o755)).Return(nil)
			cmdMock.EXPECT().RunCommand(ctx, "mount", "--rbind", "/usr/src/", "/run/mellanox/drivers/usr/src").Return("", "", nil)

			err := dm.mountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("unmountRootfs", func() {
		It("should successfully unmount when mounts exist (count > 1)", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt -r -o TARGET
			findmntOutput := "/\n/sys\n/run/mellanox/drivers/usr/src\n/run/mellanox/drivers\n/proc\n"
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return(findmntOutput, "", nil)

			// Mock umount -l -R
			cmdMock.EXPECT().RunCommand(ctx, "umount", "-l", "-R", "/run/mellanox/drivers").Return("", "", nil)

			// Mock rm -rf
			osMock.EXPECT().RemoveAll("/run/mellanox/drivers/usr/src").Return(nil)

			err := dm.unmountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip unmount when mount count is 1 or less", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt -r -o TARGET with only one mellanox occurrence
			findmntOutput := "/\n/sys\n/run/mellanox/drivers\n/proc\n"
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return(findmntOutput, "", nil)

			// Should not call umount or RemoveAll when count <= 1

			err := dm.unmountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip unmount when no mellanox mounts exist", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt -r -o TARGET without any mellanox mounts
			findmntOutput := "/\n/sys\n/proc\n/dev\n"
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return(findmntOutput, "", nil)

			// Should not call umount or RemoveAll

			err := dm.unmountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle findmnt failure gracefully", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt failing
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return("", "command not found", errors.New("findmnt failed"))

			// Should not call umount or RemoveAll and should not return error

			err := dm.unmountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error when umount fails", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt -r -o TARGET
			findmntOutput := "/\n/sys\n/run/mellanox/drivers/usr/src\n/run/mellanox/drivers\n/proc\n"
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return(findmntOutput, "", nil)

			// Mock umount failing
			cmdMock.EXPECT().RunCommand(ctx, "umount", "-l", "-R", "/run/mellanox/drivers").Return("", "target busy", errors.New("umount failed"))

			// Should return error (matches mountRootfs pattern)
			err := dm.unmountRootfs(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to unmount"))
			Expect(err.Error()).To(ContainSubstring("target busy"))
		})

		It("should return error when RemoveAll fails", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt -r -o TARGET
			findmntOutput := "/\n/sys\n/run/mellanox/drivers/usr/src\n/run/mellanox/drivers\n/proc\n"
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return(findmntOutput, "", nil)

			// Mock umount succeeding
			cmdMock.EXPECT().RunCommand(ctx, "umount", "-l", "-R", "/run/mellanox/drivers").Return("", "", nil)

			// Mock RemoveAll failing
			osMock.EXPECT().RemoveAll("/run/mellanox/drivers/usr/src").Return(errors.New("permission denied"))

			// Should return error (matches mountRootfs pattern)
			err := dm.unmountRootfs(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to remove directory"))
			Expect(err.Error()).To(ContainSubstring("permission denied"))
		})

		It("should return error when umount fails (RemoveAll not called)", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt -r -o TARGET
			findmntOutput := "/\n/sys\n/run/mellanox/drivers/usr/src\n/run/mellanox/drivers\n/proc\n"
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return(findmntOutput, "", nil)

			// Mock umount failing - this will cause early return, RemoveAll won't be called
			cmdMock.EXPECT().RunCommand(ctx, "umount", "-l", "-R", "/run/mellanox/drivers").Return("", "target busy", errors.New("umount failed"))

			// Should return error on first failure (matches mountRootfs pattern)
			err := dm.unmountRootfs(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to unmount"))
		})

		It("should count multiple mellanox mount entries correctly", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt with 3 mellanox mount entries
			findmntOutput := "/\n/run/mellanox/drivers\n/run/mellanox/drivers/usr/src\n/run/mellanox/drivers/lib\n/sys\n"
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return(findmntOutput, "", nil)

			// Should unmount since count (3) > 1
			cmdMock.EXPECT().RunCommand(ctx, "umount", "-l", "-R", "/run/mellanox/drivers").Return("", "", nil)
			osMock.EXPECT().RemoveAll("/run/mellanox/drivers/usr/src").Return(nil)

			err := dm.unmountRootfs(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Clear", func() {
		It("should call unmountRootfs", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Mock findmnt (for unmountRootfs)
			findmntOutput := "/\n/sys\n/proc\n"
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return(findmntOutput, "", nil)

			err := dm.Clear(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should propagate unmountRootfs errors", func() {
			cfg.MlxDriversMount = "/run/mellanox/drivers"
			cfg.SharedKernelHeadersDir = "/usr/src/"
			dm = New(constants.DriverContainerModePrecompiled, cfg, cmdMock, hostMock, osMock).(*driverMgr)

			// Note: unmountRootfs never returns errors currently (all are non-fatal)
			// But we test the pattern for completeness
			cmdMock.EXPECT().RunCommand(ctx, "findmnt", "-r", "-o", "TARGET").Return("", "", nil)

			err := dm.Clear(ctx)
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
