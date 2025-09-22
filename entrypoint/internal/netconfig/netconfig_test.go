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

package netconfig

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/vishvananda/netlink"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"

	netlinkMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/netconfig/netlink/mocks"
	sriovnetMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/netconfig/sriovnet/mocks"
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
	hostMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host/mocks"
	osMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers/mocks"
)

// mockLink implements netlink.Link for testing
type mockLink struct {
	attrs *netlink.LinkAttrs
}

func (m *mockLink) Attrs() *netlink.LinkAttrs {
	return m.attrs
}

func (m *mockLink) Type() string {
	return "mock"
}

var _ = Describe("Netconfig", func() {
	Context("New", func() {
		It("should create a new netconfig instance", func() {
			cmdMock := cmdMockPkg.NewInterface(GinkgoT())
			osMock := osMockPkg.NewOSWrapper(GinkgoT())
			hostMock := hostMockPkg.NewInterface(GinkgoT())
			sriovnetMock := sriovnetMockPkg.NewLib(GinkgoT())

			netlinkMock := netlinkMockPkg.NewLib(GinkgoT())
			netconfig := New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock)
			Expect(netconfig).NotTo(BeNil())
		})
	})

	Context("Save", func() {
		var (
			nc           *netconfig
			cmdMock      *cmdMockPkg.Interface
			osMock       *osMockPkg.OSWrapper
			hostMock     *hostMockPkg.Interface
			sriovnetMock *sriovnetMockPkg.Lib
			netlinkMock  *netlinkMockPkg.Lib
			ctx          context.Context
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			sriovnetMock = sriovnetMockPkg.NewLib(GinkgoT())
			netlinkMock = netlinkMockPkg.NewLib(GinkgoT())
			nc = New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock).(*netconfig)
			ctx = context.Background()
		})

		It("should succeed when mlx5_core is not loaded", func() {
			hostMock.On("LsMod", mock.Anything).Return(map[string]host.LoadedModule{}, nil).Once()

			err := nc.Save(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail when LsMod returns error", func() {
			hostMock.On("LsMod", mock.Anything).Return(nil, fmt.Errorf("lsmod failed")).Once()

			err := nc.Save(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to check if mlx5_core is loaded"))
		})

		It("should succeed when mlx5_core is loaded but no devices found", func() {
			hostMock.On("LsMod", mock.Anything).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil).Once()
			osMock.On("ReadDir", "/sys/class/net").Return([]os.DirEntry{}, nil).Once()

			err := nc.Save(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed when mlx5_core is loaded and devices are found", func() {
			// Mock LsMod to return mlx5_core as loaded
			hostMock.On("LsMod", mock.Anything).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil).Once()

			// Mock device discovery
			entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
			osMock.On("ReadDir", "/sys/class/net").Return(entries, nil).Once()

			// Mock device vendor check
			osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()

			// Mock sriovnet call - this is the key improvement!
			sriovnetMock.On("GetPciFromNetDevice", "eth0").Return("0000:08:00.0", nil).Once()

			// Create a mock link object
			mockLink := &mockLink{
				attrs: &netlink.LinkAttrs{
					Name:  "eth0",
					Flags: net.FlagUp,
					MTU:   1500,
				},
			}

			// Mock netlink call for device info collection
			netlinkMock.On("LinkByName", "eth0").Return(mockLink, nil).Once()

			// Mock device attributes (fallback when netlink fails)
			osMock.On("ReadFile", "/sys/class/net/eth0/flags").Return([]byte("0x1003"), nil).Maybe()
			osMock.On("ReadFile", "/sys/class/net/eth0/mtu").Return([]byte("1500"), nil).Maybe()
			osMock.On("ReadFile", "/sys/class/net/eth0/device/sriov_numvfs").Return([]byte("0"), nil).Once()

			// Mock devlink command
			cmdMock.On("RunCommand", mock.Anything, "devlink", "dev", "eswitch", "show", mock.Anything).Return("mode legacy", "", nil).Once()

			err := nc.Save(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle sriovnet GetPciFromNetDevice error gracefully", func() {
			// Mock LsMod to return mlx5_core as loaded
			hostMock.On("LsMod", mock.Anything).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil).Once()

			// Mock device discovery
			entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
			osMock.On("ReadDir", "/sys/class/net").Return(entries, nil).Once()

			// Mock device vendor check
			osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()

			// Mock sriovnet call to return error - this tests error handling!
			sriovnetMock.On("GetPciFromNetDevice", "eth0").Return("", fmt.Errorf("PCI address not found")).Once()

			err := nc.Save(ctx)
			Expect(err).NotTo(HaveOccurred()) // Should continue gracefully
		})

		It("should fail when device discovery fails", func() {
			hostMock.On("LsMod", mock.Anything).Return(map[string]host.LoadedModule{
				"mlx5_core": {Name: "mlx5_core", RefCount: 1, UsedBy: []string{}},
			}, nil).Once()
			osMock.On("ReadDir", "/sys/class/net").Return(nil, fmt.Errorf("readdir failed")).Once()

			err := nc.Save(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to discover Mellanox devices"))
		})
	})

	Context("Restore", func() {
		var (
			nc           *netconfig
			cmdMock      *cmdMockPkg.Interface
			osMock       *osMockPkg.OSWrapper
			hostMock     *hostMockPkg.Interface
			sriovnetMock *sriovnetMockPkg.Lib
			netlinkMock  *netlinkMockPkg.Lib
			ctx          context.Context
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			sriovnetMock = sriovnetMockPkg.NewLib(GinkgoT())
			netlinkMock = netlinkMockPkg.NewLib(GinkgoT())
			nc = New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock).(*netconfig)
			ctx = context.Background()
		})

		It("should succeed when no devices to restore", func() {
			err := nc.Restore(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed when device has no VFs", func() {
			device := &MellanoxDevice{
				PCIAddr:     "0000:08:00.0",
				DevType:     devTypeEth,
				AdminState:  adminStateUp,
				MTU:         1500,
				GUID:        "-",
				EswitchMode: eswitchModeLegacy,
				PfNumVfs:    0,
				VFs:         []VF{},
			}
			nc.mellanoxDevices["eth0"] = device

			err := nc.Restore(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Helper functions", func() {
		var (
			nc           *netconfig
			cmdMock      *cmdMockPkg.Interface
			osMock       *osMockPkg.OSWrapper
			hostMock     *hostMockPkg.Interface
			sriovnetMock *sriovnetMockPkg.Lib
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			sriovnetMock = sriovnetMockPkg.NewLib(GinkgoT())
			netlinkMock := netlinkMockPkg.NewLib(GinkgoT())
			nc = New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock).(*netconfig)
		})

		Context("getCurrentDeviceName", func() {
			It("should return device name when found", func() {
				entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
				osMock.On("ReadDir", "/sys/bus/pci/devices/0000:08:00.0/net").Return(entries, nil).Once()

				devName, err := nc.getCurrentDeviceName("0000:08:00.0")
				Expect(err).NotTo(HaveOccurred())
				Expect(devName).To(Equal("eth0"))
			})

			It("should return error when no netdev found", func() {
				osMock.On("ReadDir", "/sys/bus/pci/devices/0000:08:00.0/net").Return([]os.DirEntry{}, nil).Once()

				_, err := nc.getCurrentDeviceName("0000:08:00.0")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no netdev found for PCI address"))
			})

			It("should return error when ReadDir fails", func() {
				osMock.On("ReadDir", "/sys/bus/pci/devices/0000:08:00.0/net").Return(nil, fmt.Errorf("readdir failed")).Once()

				_, err := nc.getCurrentDeviceName("0000:08:00.0")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("setEswitchMode", func() {
			It("should succeed", func() {
				cmdMock.On("RunCommand", mock.Anything, "devlink", "dev", "eswitch", "set", "pci/0000:08:00.0", "mode", "legacy").Return("", "", nil).Once()

				err := nc.setEswitchMode(context.Background(), "0000:08:00.0", "legacy")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fail when command fails", func() {
				cmdMock.On("RunCommand", mock.Anything, "devlink", "dev", "eswitch", "set", "pci/0000:08:00.0", "mode", "legacy").Return("", "error", fmt.Errorf("devlink failed")).Once()

				err := nc.setEswitchMode(context.Background(), "0000:08:00.0", "legacy")
				Expect(err).To(HaveOccurred())
			})
		})

		Context("createVFs", func() {
			It("should succeed", func() {
				osMock.On("WriteFile", "/sys/bus/pci/devices/0000:08:00.0/sriov_numvfs", []byte("4"), os.FileMode(0o644)).Return(nil).Once()

				err := nc.createVFs("0000:08:00.0", 4)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fail when WriteFile fails", func() {
				osMock.On("WriteFile", "/sys/bus/pci/devices/0000:08:00.0/sriov_numvfs", []byte("4"), os.FileMode(0o644)).Return(fmt.Errorf("write failed")).Once()

				err := nc.createVFs("0000:08:00.0", 4)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("isMellanoxDeviceByInterface", func() {
			It("should return true for Mellanox device", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()

				result := nc.isMellanoxDeviceByInterface("eth0")
				Expect(result).To(BeTrue())
			})

			It("should return false for non-Mellanox device", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x8086"), nil).Once()

				result := nc.isMellanoxDeviceByInterface("eth0")
				Expect(result).To(BeFalse())
			})

			It("should return false when ReadFile fails", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return(nil, fmt.Errorf("read failed")).Once()

				result := nc.isMellanoxDeviceByInterface("eth0")
				Expect(result).To(BeFalse())
			})
		})

		Context("isRepresentor", func() {
			It("should return true for representor", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/phys_port_name").Return([]byte("pf0vf0"), nil).Once()

				result := nc.isRepresentor("eth0")
				Expect(result).To(BeTrue())
			})

			It("should return false for non-representor", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/phys_port_name").Return([]byte("p0"), nil).Once()

				result := nc.isRepresentor("eth0")
				Expect(result).To(BeFalse())
			})

			It("should return false when ReadFile fails", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/phys_port_name").Return(nil, fmt.Errorf("read failed")).Once()

				result := nc.isRepresentor("eth0")
				Expect(result).To(BeFalse())
			})
		})

		Context("getAdminStateFromSysfs", func() {
			It("should return up for device with up flag", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/flags").Return([]byte("0x1003"), nil).Once()

				result := nc.getAdminStateFromSysfs("eth0")
				Expect(result).To(Equal(adminStateUp))
			})

			It("should return down for device with down flag", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/flags").Return([]byte("0x1002"), nil).Once()

				result := nc.getAdminStateFromSysfs("eth0")
				Expect(result).To(Equal(adminStateDown))
			})

			It("should return down when ReadFile fails", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/flags").Return(nil, fmt.Errorf("read failed")).Once()

				result := nc.getAdminStateFromSysfs("eth0")
				Expect(result).To(Equal(adminStateDown))
			})
		})

		Context("getMTUFromSysfs", func() {
			It("should return MTU value", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/mtu").Return([]byte("1500"), nil).Once()

				result := nc.getMTUFromSysfs("eth0")
				Expect(result).To(Equal(1500))
			})

			It("should return default MTU when ReadFile fails", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/mtu").Return(nil, fmt.Errorf("read failed")).Once()

				result := nc.getMTUFromSysfs("eth0")
				Expect(result).To(Equal(1500))
			})
		})

		Context("getPfNumVfsFromSysfs", func() {
			It("should return number of VFs", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/device/sriov_numvfs").Return([]byte("4"), nil).Once()

				result := nc.getPfNumVfsFromSysfs("eth0")
				Expect(result).To(Equal(4))
			})

			It("should return 0 when ReadFile fails", func() {
				osMock.On("ReadFile", "/sys/class/net/eth0/device/sriov_numvfs").Return(nil, fmt.Errorf("read failed")).Once()

				result := nc.getPfNumVfsFromSysfs("eth0")
				Expect(result).To(Equal(0))
			})
		})

		Context("restructureGUID", func() {
			It("should restructure valid GUID", func() {
				result := nc.restructureGUID("0c42a1030016054c")
				Expect(result).To(Equal("0c42:a103:0016:054c"))
			})

			It("should return original for short GUID", func() {
				result := nc.restructureGUID("0c42a103")
				Expect(result).To(Equal("0c42a103"))
			})

			It("should return empty for empty GUID", func() {
				result := nc.restructureGUID("")
				Expect(result).To(Equal(""))
			})
		})
	})
})

// mockDirEntry is a mock implementation of os.DirEntry for testing
type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string {
	return m.name
}

func (m *mockDirEntry) IsDir() bool {
	return m.isDir
}

func (m *mockDirEntry) Type() os.FileMode {
	return 0
}

func (m *mockDirEntry) Info() (os.FileInfo, error) {
	return nil, nil
}
