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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
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
