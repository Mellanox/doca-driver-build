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

				err := dm.PreStart(ctx)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fail when driver inventory path is not a directory", func() {
				inventoryFile := filepath.Join(tempDir, "inventory")
				Expect(os.WriteFile(inventoryFile, []byte("test"), 0644)).To(Succeed())
				cfg.NvidiaNicDriversInventoryPath = inventoryFile
				dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

				err := dm.PreStart(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("NVIDIA_NIC_DRIVERS_INVENTORY_PATH is not a dir"))
			})

			It("should fail when driver inventory path is not accessible", func() {
				cfg.NvidiaNicDriversInventoryPath = "/nonexistent/path"
				dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

				hostMock.EXPECT().GetOSType(ctx).Return(constants.OSTypeUbuntu, nil)

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

	Context("Load", func() {
		BeforeEach(func() {
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)
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

			result, err := dm.Load(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
			Expect(dm.newDriverLoaded).To(BeTrue())
		})

		It("should include NFS RDMA modules when enabled", func() {
			cfg.EnableNfsRdma = true
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

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
			dm = New(constants.DriverContainerModeSources, cfg, cmdMock, hostMock, osMock).(*driverMgr)

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
})
