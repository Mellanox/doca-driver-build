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
			netconfig := New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock, 4)
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
			nc = New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock, 4).(*netconfig)
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
			osMock.On("ReadDir", "/sys/class/net/").Return([]os.DirEntry{}, nil).Once()

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
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

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
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

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
			osMock.On("ReadDir", "/sys/class/net/").Return(nil, fmt.Errorf("readdir failed")).Once()

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
			nc = New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock, 4).(*netconfig)
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
			nc = New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock, 4).(*netconfig)
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

		Context("setIBGUIDs", func() {
			It("should skip invalid all-zero GUID", func() {
				// Test that invalid GUID (00:00:00:00:00:00:00:00) is skipped
				// and no ip link commands are called
				err := nc.setIBGUIDs(context.Background(), "eth0", 0, "00:00:00:00:00:00:00:00")
				Expect(err).NotTo(HaveOccurred())

				// Verify no commands were called
				cmdMock.AssertNotCalled(GinkgoT(), "RunCommand")
			})

			It("should set valid GUID successfully", func() {
				validGUID := "0c:42:a1:03:00:16:05:4c"

				// Mock successful port_guid command
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "eth0", "vf", "0", "port_guid", validGUID).
					Return("", "", nil).Once()

				// Mock successful node_guid command
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "eth0", "vf", "0", "node_guid", validGUID).
					Return("", "", nil).Once()

				err := nc.setIBGUIDs(context.Background(), "eth0", 0, validGUID)
				Expect(err).NotTo(HaveOccurred())

				// Verify both commands were called
				cmdMock.AssertExpectations(GinkgoT())
			})

			It("should return error when port_guid command fails", func() {
				validGUID := "0c:42:a1:03:00:16:05:4c"

				// Mock failed port_guid command
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "eth0", "vf", "0", "port_guid", validGUID).
					Return("", "error setting port_guid", fmt.Errorf("command failed")).Once()

				err := nc.setIBGUIDs(context.Background(), "eth0", 0, validGUID)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to set port GUID"))
			})

			It("should return error when node_guid command fails", func() {
				validGUID := "0c:42:a1:03:00:16:05:4c"

				// Mock successful port_guid command
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "eth0", "vf", "0", "port_guid", validGUID).
					Return("", "", nil).Once()

				// Mock failed node_guid command
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "eth0", "vf", "0", "node_guid", validGUID).
					Return("", "error setting node_guid", fmt.Errorf("command failed")).Once()

				err := nc.setIBGUIDs(context.Background(), "eth0", 0, validGUID)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to set node GUID"))
			})

			It("should handle different VF indices correctly", func() {
				validGUID := "0c:42:a1:03:00:16:05:4d"
				vfIndex := 3

				// Mock successful commands for VF index 3
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "eth0", "vf", "3", "port_guid", validGUID).
					Return("", "", nil).Once()
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "eth0", "vf", "3", "node_guid", validGUID).
					Return("", "", nil).Once()

				err := nc.setIBGUIDs(context.Background(), "eth0", vfIndex, validGUID)
				Expect(err).NotTo(HaveOccurred())

				cmdMock.AssertExpectations(GinkgoT())
			})
		})
	})

	Context("Switchdev Flow", func() {
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
			nc = New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock, 4).(*netconfig)
			ctx = context.Background()
		})

		Context("Helper functions for switchdev", func() {
			Context("isRepresentorPhysPortName", func() {
				It("should return true for valid representor phys port name", func() {
					result := nc.isRepresentorPhysPortName("pf1vf0")
					Expect(result).To(BeTrue())
				})

				It("should return false for invalid phys port name", func() {
					result := nc.isRepresentorPhysPortName("p0")
					Expect(result).To(BeFalse())
				})

				It("should return false for empty phys port name", func() {
					result := nc.isRepresentorPhysPortName("")
					Expect(result).To(BeFalse())
				})
			})

			Context("parseRepresentorPhysPortName", func() {
				It("should parse valid representor phys port name", func() {
					pfID, vfID, err := nc.parseRepresentorPhysPortName("pf1vf0")
					Expect(err).NotTo(HaveOccurred())
					Expect(pfID).To(Equal("1"))
					Expect(vfID).To(Equal("0"))
				})

				It("should return error for invalid format", func() {
					_, _, err := nc.parseRepresentorPhysPortName("p0")
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context("VF rebinding logic", func() {
			It("should skip VF rebinding for switchdev mode", func() {
				device := &MellanoxDevice{
					PCIAddr:     "0000:08:00.1",
					DevType:     devTypeEth,
					AdminState:  adminStateDown,
					MTU:         1500,
					GUID:        "-",
					EswitchMode: eswitchModeSwitchdev,
					PfNumVfs:    1,
					VFs: []VF{
						{VFIndex: 0, VFPCIAddr: "0000:08:01.0", VFName: "eth10", AdminState: adminStateDown, MACAddress: "2a:c1:0b:f4:b5:3e", AdminMAC: "00:00:00:00:00:00", MTU: 1500, GUID: "-"},
					},
				}

				// Mock VF configuration
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "eth3", "vf", "0", "mac", "00:00:00:00:00:00").Return("", "", nil).Once()

				// Mock VF unbinding
				osMock.On("WriteFile", "/sys/bus/pci/drivers/mlx5_core/unbind", []byte("0000:08:01.0"), os.FileMode(0o644)).Return(nil).Once()

				// Mock Readlink for driver check
				osMock.On("Readlink", "/sys/bus/pci/devices/0000:08:01.0/driver").Return("../../../../bus/pci/drivers/mlx5_core", nil).Once()

				// Mock getCurrentDeviceName for VF
				osMock.On("ReadDir", "/sys/bus/pci/devices/0000:08:01.0/net").Return([]os.DirEntry{&mockDirEntry{name: "eth10"}}, nil).Once()

				// Mock netlink call for VF state restoration
				mockLink := &mockLink{
					attrs: &netlink.LinkAttrs{
						Name:  "eth10",
						Flags: net.FlagUp,
						MTU:   1500,
					},
				}
				netlinkMock.On("LinkByName", "eth10").Return(mockLink, nil).Once()
				netlinkMock.On("LinkSetHardwareAddr", mockLink, mock.AnythingOfType("net.HardwareAddr")).Return(nil).Once()

				err := nc.restoreVFConfigurations(ctx, "eth3", device, eswitchModeSwitchdev)
				Expect(err).NotTo(HaveOccurred())

				// Verify VF was configured and unbound, but not rebound
				cmdMock.AssertExpectations(GinkgoT())
			})

			It("should rebind VFs for legacy mode", func() {
				device := &MellanoxDevice{
					PCIAddr:     "0000:08:00.0",
					DevType:     devTypeEth,
					AdminState:  adminStateUp,
					MTU:         1500,
					GUID:        "-",
					EswitchMode: eswitchModeLegacy,
					PfNumVfs:    1,
					VFs: []VF{
						{VFIndex: 0, VFPCIAddr: "0000:08:00.2", VFName: "eth4", AdminState: adminStateUp, MACAddress: "aa:bb:cc:dd:ee:01", AdminMAC: "aa:bb:cc:dd:ee:01", MTU: 1500, GUID: "-"},
					},
				}

				// Mock VF configuration
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "eth2", "vf", "0", "mac", "aa:bb:cc:dd:ee:01").Return("", "", nil).Maybe()

				// Mock VF unbinding and rebinding
				osMock.On("WriteFile", "/sys/bus/pci/drivers/mlx5_core/unbind", []byte("0000:08:00.2"), os.FileMode(0o644)).Return(nil).Maybe()
				osMock.On("WriteFile", "/sys/bus/pci/drivers/mlx5_core/bind", []byte("0000:08:00.2"), os.FileMode(0o644)).Return(nil).Maybe()

				// Mock Readlink for driver check
				osMock.On("Readlink", "/sys/bus/pci/devices/0000:08:00.2/driver").Return("../../../../bus/pci/drivers/mlx5_core", nil).Maybe()

				// Mock VF state restoration
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "eth4", "mtu", "1500").Return("", "", nil).Maybe()
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "eth4", "up").Return("", "", nil).Maybe()

				// Mock getCurrentDeviceName for VF
				osMock.On("ReadDir", "/sys/bus/pci/devices/0000:08:00.2/net").Return([]os.DirEntry{&mockDirEntry{name: "eth4"}}, nil).Maybe()

				// Mock netlink call for VF state restoration
				mockLink := &mockLink{
					attrs: &netlink.LinkAttrs{
						Name:  "eth4",
						Flags: net.FlagUp,
						MTU:   1500,
					},
				}
				netlinkMock.On("LinkByName", "eth4").Return(mockLink, nil).Maybe()
				netlinkMock.On("LinkSetHardwareAddr", mockLink, mock.AnythingOfType("net.HardwareAddr")).Return(nil).Maybe()
				netlinkMock.On("LinkSetMTU", mockLink, 1500).Return(nil).Maybe()
				netlinkMock.On("LinkSetUp", mockLink).Return(nil).Maybe()

				err := nc.restoreVFConfigurations(ctx, "eth2", device, eswitchModeLegacy)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("restoreRepresentors with two-phase rename", func() {
			It("should use two-phase rename to avoid name collisions", func() {
				// This test verifies the two-phase rename mechanism prevents name collisions
				// when interfaces are swapped after driver reload
				// Scenario: rep1 should become eth_rep0, rep0 should become eth_rep1
				// Without two-phase rename, this would fail due to name collision

				device := &MellanoxDevice{
					PCIAddr:     "0000:08:00.0",
					DevType:     devTypeEth,
					AdminState:  adminStateUp,
					MTU:         1500,
					GUID:        "-",
					EswitchMode: eswitchModeSwitchdev,
					PfNumVfs:    2,
					VFs: []VF{
						{VFIndex: 0, VFPCIAddr: "0000:08:00.2", VFName: "eth_vf0", AdminState: adminStateUp, MACAddress: "aa:bb:cc:dd:ee:01", AdminMAC: "aa:bb:cc:dd:ee:01", MTU: 1500, GUID: "-"},
						{VFIndex: 1, VFPCIAddr: "0000:08:00.3", VFName: "eth_vf1", AdminState: adminStateDown, MACAddress: "aa:bb:cc:dd:ee:02", AdminMAC: "aa:bb:cc:dd:ee:02", MTU: 9000, GUID: "-"},
					},
					Representors: []Representor{
						{PhysSwitchID: "00000000000000ab", PhysPortNum: "1", VFID: "0", Name: "eth_rep0", AdminState: adminStateUp, MTU: 1500},
						{PhysSwitchID: "00000000000000ab", PhysPortNum: "1", VFID: "1", Name: "eth_rep1", AdminState: adminStateDown, MTU: 9000},
					},
				}

				// Mock PF physical switch ID and port name
				osMock.On("ReadFile", "/sys/class/net/eth5/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
				osMock.On("ReadFile", "/sys/class/net/eth5/phys_port_name").Return([]byte("p1"), nil).Once()

				// Phase 1: Find current representors and rename to temporary names
				// For VF 0 - find current representor (currently named "rep1")
				osMock.On("ReadDir", "/sys/class/net/").Return([]os.DirEntry{
					&mockDirEntry{name: "rep0"},
					&mockDirEntry{name: "rep1"},
					&mockDirEntry{name: "lo"},
				}, nil).Once()

				// Check rep0 - this is VF 1 representor
				osMock.On("ReadFile", "/sys/class/net/rep0/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
				osMock.On("ReadFile", "/sys/class/net/rep0/phys_port_name").Return([]byte("pf1vf1"), nil).Once()

				// Check rep1 - this is VF 0 representor (matches!)
				osMock.On("ReadFile", "/sys/class/net/rep1/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
				osMock.On("ReadFile", "/sys/class/net/rep1/phys_port_name").Return([]byte("pf1vf0"), nil).Once()

				// Phase 1: Rename rep1 -> t00abp1v0 (temporary name with switch hash)
				// Switch ID "00000000000000ab" -> last 4 chars = "00ab"
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "rep1", "name", "t00abp1v0").Return("", "", nil).Once()

				// For VF 1 - find current representor (currently named "rep0")
				osMock.On("ReadDir", "/sys/class/net/").Return([]os.DirEntry{
					&mockDirEntry{name: "rep0"},
					&mockDirEntry{name: "t00abp1v0"}, // rep1 already renamed to temp
					&mockDirEntry{name: "lo"},
				}, nil).Once()

				// Check rep0 - this is VF 1 representor (matches!)
				osMock.On("ReadFile", "/sys/class/net/rep0/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
				osMock.On("ReadFile", "/sys/class/net/rep0/phys_port_name").Return([]byte("pf1vf1"), nil).Once()

				// Phase 1: Rename rep0 -> t00abp1v1 (temporary name with switch hash)
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "rep0", "name", "t00abp1v1").Return("", "", nil).Once()

				// Phase 2: Rename from temporary to final names
				// Rename t00abp1v0 -> eth_rep0
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "t00abp1v0", "name", "eth_rep0").Return("", "", nil).Once()

				// Set MTU for eth_rep0 (LinkByName called once for MTU)
				mockLink0 := &mockLink{
					attrs: &netlink.LinkAttrs{
						Name:  "eth_rep0",
						Flags: net.FlagUp,
						MTU:   1500,
					},
				}
				netlinkMock.On("LinkByName", "eth_rep0").Return(mockLink0, nil).Once()
				netlinkMock.On("LinkSetMTU", mockLink0, 1500).Return(nil).Once()

				// Set admin state for eth_rep0 (LinkByName called again for admin state)
				netlinkMock.On("LinkByName", "eth_rep0").Return(mockLink0, nil).Once()
				netlinkMock.On("LinkSetUp", mockLink0).Return(nil).Once()

				// Rename t00abp1v1 -> eth_rep1
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "t00abp1v1", "name", "eth_rep1").Return("", "", nil).Once()

				// Set MTU for eth_rep1 (LinkByName called once for MTU)
				mockLink1 := &mockLink{
					attrs: &netlink.LinkAttrs{
						Name:  "eth_rep1",
						Flags: 0, // Down
						MTU:   9000,
					},
				}
				netlinkMock.On("LinkByName", "eth_rep1").Return(mockLink1, nil).Once()
				netlinkMock.On("LinkSetMTU", mockLink1, 9000).Return(nil).Once()

				// Set admin state for eth_rep1 (LinkByName called again for admin state)
				netlinkMock.On("LinkByName", "eth_rep1").Return(mockLink1, nil).Once()
				netlinkMock.On("LinkSetDown", mockLink1).Return(nil).Once()

				err := nc.restoreRepresentors(ctx, "eth5", device)
				Expect(err).NotTo(HaveOccurred())

				// Verify all expectations were met
				cmdMock.AssertExpectations(GinkgoT())
				osMock.AssertExpectations(GinkgoT())
				netlinkMock.AssertExpectations(GinkgoT())
			})

			It("should handle error in Phase 1 rename gracefully", func() {
				device := &MellanoxDevice{
					PCIAddr:     "0000:08:00.0",
					DevType:     devTypeEth,
					AdminState:  adminStateUp,
					MTU:         1500,
					GUID:        "-",
					EswitchMode: eswitchModeSwitchdev,
					PfNumVfs:    1,
					VFs: []VF{
						{VFIndex: 0, VFPCIAddr: "0000:08:00.2", VFName: "eth_vf0", AdminState: adminStateUp, MACAddress: "aa:bb:cc:dd:ee:01", AdminMAC: "aa:bb:cc:dd:ee:01", MTU: 1500, GUID: "-"},
					},
					Representors: []Representor{
						{PhysSwitchID: "00000000000000ab", PhysPortNum: "1", VFID: "0", Name: "eth_rep0", AdminState: adminStateUp, MTU: 1500},
					},
				}

				// Mock PF physical switch ID and port name
				osMock.On("ReadFile", "/sys/class/net/eth5/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
				osMock.On("ReadFile", "/sys/class/net/eth5/phys_port_name").Return([]byte("p1"), nil).Once()

				// Find current representor
				osMock.On("ReadDir", "/sys/class/net/").Return([]os.DirEntry{
					&mockDirEntry{name: "rep0"},
					&mockDirEntry{name: "lo"},
				}, nil).Once()

				osMock.On("ReadFile", "/sys/class/net/rep0/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
				osMock.On("ReadFile", "/sys/class/net/rep0/phys_port_name").Return([]byte("pf1vf0"), nil).Once()

				// Phase 1: Rename fails
				cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", "rep0", "name", "t00abp1v0").
					Return("", "device busy", fmt.Errorf("rename failed")).Once()

				// Phase 2 should not execute since Phase 1 failed (no renameOps added)
				// No additional mock expectations needed - Phase 2 won't run

				// The function should continue despite the error
				err := nc.restoreRepresentors(ctx, "eth5", device)
				Expect(err).NotTo(HaveOccurred())

				cmdMock.AssertExpectations(GinkgoT())
				osMock.AssertExpectations(GinkgoT())
			})

			It("should generate correct temporary names for different ports and VFs", func() {
				// This test verifies the temporary name format: t<switch_hash>p<port>v<vf>
				device := &MellanoxDevice{
					PCIAddr:     "0000:08:00.0",
					DevType:     devTypeEth,
					AdminState:  adminStateUp,
					MTU:         1500,
					GUID:        "-",
					EswitchMode: eswitchModeSwitchdev,
					PfNumVfs:    3,
					VFs: []VF{
						{VFIndex: 0, VFPCIAddr: "0000:08:00.2", VFName: "eth_vf0", AdminState: adminStateUp, MACAddress: "aa:bb:cc:dd:ee:01", AdminMAC: "aa:bb:cc:dd:ee:01", MTU: 1500, GUID: "-"},
						{VFIndex: 1, VFPCIAddr: "0000:08:00.3", VFName: "eth_vf1", AdminState: adminStateUp, MACAddress: "aa:bb:cc:dd:ee:02", AdminMAC: "aa:bb:cc:dd:ee:02", MTU: 1500, GUID: "-"},
						{VFIndex: 2, VFPCIAddr: "0000:08:00.4", VFName: "eth_vf2", AdminState: adminStateUp, MACAddress: "aa:bb:cc:dd:ee:03", AdminMAC: "aa:bb:cc:dd:ee:03", MTU: 1500, GUID: "-"},
					},
					Representors: []Representor{
						{PhysSwitchID: "00000000000000ab", PhysPortNum: "2", VFID: "0", Name: "final_rep0", AdminState: adminStateUp, MTU: 1500},
						{PhysSwitchID: "00000000000000ab", PhysPortNum: "2", VFID: "1", Name: "final_rep1", AdminState: adminStateUp, MTU: 1500},
						{PhysSwitchID: "00000000000000ab", PhysPortNum: "2", VFID: "2", Name: "final_rep2", AdminState: adminStateUp, MTU: 1500},
					},
				}

				// Mock PF physical switch ID and port name - port 2
				osMock.On("ReadFile", "/sys/class/net/eth5/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
				osMock.On("ReadFile", "/sys/class/net/eth5/phys_port_name").Return([]byte("p2"), nil).Once()

				// Expected temporary names: t00abp2v0, t00abp2v1, t00abp2v2 (switch_hash=00ab, port=2, vf=0,1,2)

				// Mock finding and renaming each representor to temp names (Phase 1)
				// For each representor, findCurrentRepresentor scans all devices
				for i := 0; i < 3; i++ {
					currentName := fmt.Sprintf("rep%d", i)
					tempName := fmt.Sprintf("t00abp2v%d", i) // switch_hash=00ab, port=2, vf=i

					// Mock ReadDir for findCurrentRepresentor
					osMock.On("ReadDir", "/sys/class/net/").Return([]os.DirEntry{
						&mockDirEntry{name: "rep0"},
						&mockDirEntry{name: "rep1"},
						&mockDirEntry{name: "rep2"},
						&mockDirEntry{name: "lo"},
					}, nil).Once()

					// findCurrentRepresentor will check each device until it finds the match
					// It checks rep0 first
					osMock.On("ReadFile", "/sys/class/net/rep0/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
					osMock.On("ReadFile", "/sys/class/net/rep0/phys_port_name").Return([]byte("pf2vf0"), nil).Once()

					// If looking for VF 0, it matches on rep0, so we stop here
					// If looking for VF 1 or 2, we continue checking
					if i > 0 {
						// Check rep1
						osMock.On("ReadFile", "/sys/class/net/rep1/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
						osMock.On("ReadFile", "/sys/class/net/rep1/phys_port_name").Return([]byte("pf2vf1"), nil).Once()
					}

					if i > 1 {
						// Check rep2
						osMock.On("ReadFile", "/sys/class/net/rep2/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
						osMock.On("ReadFile", "/sys/class/net/rep2/phys_port_name").Return([]byte("pf2vf2"), nil).Once()
					}

					// Phase 1: Rename to temp name
					cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", currentName, "name", tempName).Return("", "", nil).Once()
				}

				// Mock Phase 2: Rename from temp to final names and set configs
				for i := 0; i < 3; i++ {
					tempName := fmt.Sprintf("t00abp2v%d", i)
					finalName := fmt.Sprintf("final_rep%d", i)

					cmdMock.On("RunCommand", mock.Anything, "ip", "link", "set", "dev", tempName, "name", finalName).Return("", "", nil).Once()

					mockLink := &mockLink{
						attrs: &netlink.LinkAttrs{
							Name:  finalName,
							Flags: net.FlagUp,
							MTU:   1500,
						},
					}
					// LinkByName called twice per representor (once for MTU, once for admin state)
					netlinkMock.On("LinkByName", finalName).Return(mockLink, nil).Once()
					netlinkMock.On("LinkSetMTU", mockLink, 1500).Return(nil).Once()
					netlinkMock.On("LinkByName", finalName).Return(mockLink, nil).Once()
					netlinkMock.On("LinkSetUp", mockLink).Return(nil).Once()
				}

				err := nc.restoreRepresentors(ctx, "eth5", device)
				Expect(err).NotTo(HaveOccurred())

				cmdMock.AssertExpectations(GinkgoT())
				osMock.AssertExpectations(GinkgoT())
				netlinkMock.AssertExpectations(GinkgoT())
			})

			It("should handle representor not found error", func() {
				device := &MellanoxDevice{
					PCIAddr:     "0000:08:00.0",
					DevType:     devTypeEth,
					AdminState:  adminStateUp,
					MTU:         1500,
					GUID:        "-",
					EswitchMode: eswitchModeSwitchdev,
					PfNumVfs:    1,
					VFs: []VF{
						{VFIndex: 0, VFPCIAddr: "0000:08:00.2", VFName: "eth_vf0", AdminState: adminStateUp, MACAddress: "aa:bb:cc:dd:ee:01", AdminMAC: "aa:bb:cc:dd:ee:01", MTU: 1500, GUID: "-"},
					},
					Representors: []Representor{
						{PhysSwitchID: "00000000000000ab", PhysPortNum: "1", VFID: "0", Name: "eth_rep0", AdminState: adminStateUp, MTU: 1500},
					},
				}

				// Mock PF physical switch ID and port name
				osMock.On("ReadFile", "/sys/class/net/eth5/phys_switch_id").Return([]byte("00000000000000ab"), nil).Once()
				osMock.On("ReadFile", "/sys/class/net/eth5/phys_port_name").Return([]byte("p1"), nil).Once()

				// Mock finding representor - but none found
				osMock.On("ReadDir", "/sys/class/net/").Return([]os.DirEntry{
					&mockDirEntry{name: "lo"},
				}, nil).Once()

				// Should continue without error
				err := nc.restoreRepresentors(ctx, "eth5", device)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("DevicesUseNewNamingScheme", func() {
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
			nc = New(cmdMock, osMock, hostMock, sriovnetMock, netlinkMock, 4).(*netconfig)
			ctx = context.Background()
		})
		It("should return true when device uses new naming scheme (np suffix)", func() {
			// Mock device discovery - return one NVIDIA device
			entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

			// Mock device vendor check - NVIDIA device
			osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()

			// Mock udevadm command - return NetNamePath with np suffix (new naming scheme)
			cmdMock.On("RunCommand", mock.Anything, "udevadm", "info", "--query=property", "/sys/class/net/eth0").Return("ID_NET_NAME_PATH=pci-0000:08:00.0np0", "", nil).Once()

			result, err := nc.DevicesUseNewNamingScheme(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})

		It("should return false when device uses old naming scheme (no np suffix)", func() {
			// Mock device discovery - return one NVIDIA device
			entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

			// Mock device vendor check - NVIDIA device
			osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()

			// Mock udevadm command - return NetNamePath without np suffix (old naming scheme)
			cmdMock.On("RunCommand", mock.Anything, "udevadm", "info", "--query=property", "/sys/class/net/eth0").Return("ID_NET_NAME_PATH=pci-0000:08:00.0", "", nil).Once()

			result, err := nc.DevicesUseNewNamingScheme(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should return false when no NVIDIA devices are found", func() {
			// Mock device discovery - return one non-NVIDIA device
			entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

			// Mock device vendor check - non-NVIDIA device
			osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x8086"), nil).Once()

			result, err := nc.DevicesUseNewNamingScheme(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should return false when no devices are found", func() {
			// Mock device discovery - return empty list
			entries := []os.DirEntry{}
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

			result, err := nc.DevicesUseNewNamingScheme(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should handle multiple devices and return true if any uses new naming scheme", func() {
			// Mock device discovery - return multiple devices
			entries := []os.DirEntry{
				&mockDirEntry{name: "eth0"},
				&mockDirEntry{name: "eth1"},
			}
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

			// Mock device vendor checks - both NVIDIA devices
			osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()
			osMock.On("ReadFile", "/sys/class/net/eth1/device/vendor").Return([]byte("0x15b3"), nil).Once()

			// Mock udevadm commands - first device uses old scheme, second uses new scheme
			cmdMock.On("RunCommand", mock.Anything, "udevadm", "info", "--query=property", "/sys/class/net/eth0").Return("ID_NET_NAME_PATH=pci-0000:08:00.0", "", nil).Once()
			cmdMock.On("RunCommand", mock.Anything, "udevadm", "info", "--query=property", "/sys/class/net/eth1").Return("ID_NET_NAME_PATH=pci-0000:08:00.1np1", "", nil).Once()

			result, err := nc.DevicesUseNewNamingScheme(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeTrue())
		})

		It("should handle udevadm command failure gracefully", func() {
			// Mock device discovery - return one NVIDIA device
			entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

			// Mock device vendor check - NVIDIA device
			osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()

			// Mock udevadm command failure
			cmdMock.On("RunCommand", mock.Anything, "udevadm", "info", "--query=property", "/sys/class/net/eth0").Return("", "command failed", fmt.Errorf("udevadm failed")).Once()

			result, err := nc.DevicesUseNewNamingScheme(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should handle missing ID_NET_NAME_PATH in udevadm output", func() {
			// Mock device discovery - return one NVIDIA device
			entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
			osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

			// Mock device vendor check - NVIDIA device
			osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()

			// Mock udevadm command - return output without ID_NET_NAME_PATH
			cmdMock.On("RunCommand", mock.Anything, "udevadm", "info", "--query=property", "/sys/class/net/eth0").Return("OTHER_PROPERTY=value", "", nil).Once()

			result, err := nc.DevicesUseNewNamingScheme(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeFalse())
		})

		It("should handle ReadDir failure", func() {
			// Mock ReadDir failure
			osMock.On("ReadDir", "/sys/class/net/").Return(nil, fmt.Errorf("readdir failed")).Once()

			result, err := nc.DevicesUseNewNamingScheme(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("readdir failed"))
			Expect(result).To(BeFalse())
		})

		It("should handle different np patterns (np0, np1, np2, np3)", func() {
			testCases := []struct {
				netNamePath string
				expected    bool
			}{
				{"pci-0000:08:00.0np0", true},
				{"pci-0000:08:00.0np1", true},
				{"pci-0000:08:00.0np2", true},
				{"pci-0000:08:00.0np3", true},
				{"pci-0000:08:00.0", false},
				{"pci-0000:08:00.0np4", false}, // np4 should not match
				{"pci-0000:08:00.0np", false},  // incomplete np
			}

			for _, tc := range testCases {
				// Mock device discovery
				entries := []os.DirEntry{&mockDirEntry{name: "eth0"}}
				osMock.On("ReadDir", "/sys/class/net/").Return(entries, nil).Once()

				// Mock device vendor check
				osMock.On("ReadFile", "/sys/class/net/eth0/device/vendor").Return([]byte("0x15b3"), nil).Once()

				// Mock udevadm command with specific NetNamePath
				cmdMock.On("RunCommand", mock.Anything, "udevadm", "info", "--query=property", "/sys/class/net/eth0").Return(fmt.Sprintf("ID_NET_NAME_PATH=%s", tc.netNamePath), "", nil).Once()

				result, err := nc.DevicesUseNewNamingScheme(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(tc.expected), "NetNamePath: %s should return %v", tc.netNamePath, tc.expected)
			}
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
