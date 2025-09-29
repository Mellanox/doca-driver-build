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
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/netconfig/netlink"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/netconfig/sriovnet"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

// Constants for device types and states
const (
	devTypeIB            = "ib"
	devTypeEth           = "eth"
	adminStateUp         = "up"
	adminStateDown       = "down"
	eswitchModeLegacy    = "legacy"
	eswitchModeSwitchdev = "switchdev"
	sysClassNetPath      = "/sys/class/net/"
	sysBusPCIDevicesPath = "/sys/bus/pci/devices/"
	sysBusPCIDriversPath = "/sys/bus/pci/drivers/"
	defaultDriverPath    = sysBusPCIDriversPath + "mlx5_core"
)

// JSON structures for parsing ip command output
type VFInfo struct {
	Address  string `json:"address"`
	PortGUID string `json:"port guid"`
}

type LinkInfo struct {
	VFinfoList []VFInfo `json:"vfinfo_list"`
}

// New initialize default implementation of the netconfig.Interface.
func New(
	cmdHelper cmd.Interface,
	osWrapper wrappers.OSWrapper,
	hostHelper host.Interface,
	sriovnetLib sriovnet.Lib,
	netlinkLib netlink.Lib,
) Interface {
	return &netconfig{
		cmd:             cmdHelper,
		os:              osWrapper,
		host:            hostHelper,
		sriovnetLib:     sriovnetLib,
		netlinkLib:      netlinkLib,
		mellanoxDevices: make(map[string]*MellanoxDevice),
	}
}

// Interface is the interface exposed by the netconfig package.
type Interface interface {
	// Save function preserves the current NVIDIA network configuration,
	// allowing it to be restored after a driver reload.
	// It supports PF, VF, and VF representor configurations.
	Save(ctx context.Context) error
	// Restore the saved configuration for NVIDIA devices.
	Restore(ctx context.Context) error
	// DevicesUseNewNamingScheme returns true if interfaces with the new naming scheme
	// are on the host or if no NVIDIA devices are found.
	DevicesUseNewNamingScheme(ctx context.Context) (bool, error)
}

// VF represents a Virtual Function with all its attributes
type VF struct {
	// VF identification
	VFIndex   int    // VF index (0-based)
	VFPCIAddr string // VF PCI address (e.g., "0000:08:00.2")
	VFName    string // VF netdev name (e.g., "eth6")

	// VF configuration
	AdminState string // VF admin state: "up" or "down"
	MACAddress string // VF hardware MAC address
	AdminMAC   string // VF administrative MAC address
	MTU        int    // VF MTU value
	GUID       string // VF GUID (for IB) or "-" for Ethernet
}

// Representor represents a switchdev representor device
type Representor struct {
	// Representor identification
	PhysSwitchID string // Physical switch ID
	PhysPortNum  string // Physical port number
	VFID         string // VF ID
	Name         string // Representor netdev name

	// Representor configuration
	AdminState string // Representor admin state: "up" or "down"
	MTU        int    // Representor MTU value
}

// MellanoxDevice represents a Mellanox network device with all its attributes
type MellanoxDevice struct {
	// Basic device information
	PCIAddr     string // PCI address (e.g., "0000:08:00.0")
	DevType     string // Device type: "eth" or "ib"
	AdminState  string // Admin state: "up" or "down"
	MTU         int    // MTU value
	GUID        string // Device GUID (for IB) or "-" for Ethernet
	EswitchMode string // Eswitch mode: "legacy" or "switchdev"

	// SRIOV information
	PfNumVfs     int           // Number of VFs configured (from sriov_numvfs)
	VFs          []VF          // Array of VF information
	Representors []Representor // Array of representor information (for switchdev mode)
}

type netconfig struct {
	cmd         cmd.Interface
	os          wrappers.OSWrapper
	host        host.Interface
	sriovnetLib sriovnet.Lib
	netlinkLib  netlink.Lib

	// In-memory storage - Mellanox device information
	mellanoxDevices map[string]*MellanoxDevice
}

// Save discovers and stores the current SRIOV configuration
func (n *netconfig) Save(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Saving SRIOV configuration")

	// Check if mlx5_core driver is loaded
	mlx5CoreLoaded, err := n.isMlx5CoreLoaded(ctx)
	if err != nil {
		return fmt.Errorf("failed to check if mlx5_core is loaded: %w", err)
	}

	if !mlx5CoreLoaded {
		log.Info("mlx5_core driver not loaded, skipped store netdev conf info")
		return nil
	}

	// Clear existing configuration
	n.mellanoxDevices = make(map[string]*MellanoxDevice)

	// Discover Mellanox devices
	devices, err := n.discoverMellanoxDevices(ctx)
	if err != nil {
		return fmt.Errorf("failed to discover Mellanox devices: %w", err)
	}

	if len(devices) == 0 {
		log.Info("No Mellanox devices found, skipping SRIOV configuration")
		return nil
	}

	// Discover switchdev representors for devices in switchdev mode
	if err := n.discoverSwitchdevRepresentors(ctx); err != nil {
		log.Error(err, "Failed to discover switchdev representors")
		return fmt.Errorf("failed to discover switchdev representors: %w", err)
	}

	log.Info("SRIOV configuration saved successfully", "devices", len(n.mellanoxDevices))
	return nil
}

// Restore restores the saved SRIOV configuration
func (n *netconfig) Restore(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Restoring SRIOV configuration")

	if len(n.mellanoxDevices) == 0 {
		log.Info("No SRIOV configuration to restore")
		return nil
	}

	// Restore each device
	for devName, device := range n.mellanoxDevices {
		log.Info("Restoring SRIOV config for device", "device", devName, "vfs", device.PfNumVfs)

		// Skip devices with no VFs configured
		if device.PfNumVfs == 0 {
			log.V(1).Info("Device has no VFs configured, skipping", "device", devName)
			continue
		}

		// Restore PF and VF configuration
		if err := n.restoreDeviceConfig(ctx, devName, device); err != nil {
			log.Error(err, "Failed to restore device config", "device", devName)
			continue
		}

		log.Info("Successfully restored SRIOV config for device", "device", devName, "vfs", device.PfNumVfs)
	}

	log.Info("SRIOV configuration restored successfully")
	return nil
}

// restoreDeviceConfig restores the configuration for a single device and its VFs
func (n *netconfig) restoreDeviceConfig(ctx context.Context, devName string, device *MellanoxDevice) error {
	log := logr.FromContextOrDiscard(ctx)

	// Get the current device name (might have changed after driver reload)
	currentDevName, err := n.getCurrentDeviceName(device.PCIAddr)
	if err != nil {
		return fmt.Errorf("failed to get current device name: %w", err)
	}

	log.Info("Restoring device config", "original_name", devName, "current_name", currentDevName, "pci", device.PCIAddr)

	// Handle switchdev mode (set to legacy first if needed)
	// To support the old kernel versions, we need to follow the recommended way of creating switchdev VFs
	// 1) Set the NIC in legacy mode
	// 2) Create the required amount of VFs
	// 3) Unbind all of the VFs
	// 4) Set the NIC in switchdev mode
	if device.EswitchMode == eswitchModeSwitchdev {
		if err := n.setEswitchMode(ctx, device.PCIAddr, eswitchModeLegacy); err != nil {
			log.Error(err, "Failed to set eswitch mode to legacy", "device", currentDevName)
			return err
		}
	}

	// Restore PF admin state
	if err := n.setDeviceAdminState(currentDevName, device.AdminState); err != nil {
		log.Error(err, "Failed to set PF admin state", "device", currentDevName, "state", device.AdminState)
		return err
	}

	// Create VFs
	if err := n.createVFs(device.PCIAddr, device.PfNumVfs); err != nil {
		log.Error(err, "Failed to create VFs", "device", currentDevName, "vfs", device.PfNumVfs)
		return err
	}

	// Restore VF configurations (but don't rebind VFs if in switchdev mode)
	if err := n.restoreVFConfigurations(ctx, currentDevName, device, device.EswitchMode); err != nil {
		log.Error(err, "Failed to restore VF configurations", "device", currentDevName)
		return err
	}

	// Set switchdev mode if needed
	if device.EswitchMode == eswitchModeSwitchdev {
		if err := n.setEswitchMode(ctx, device.PCIAddr, eswitchModeSwitchdev); err != nil {
			log.Error(err, "Failed to set eswitch mode to switchdev", "device", currentDevName)
			return err
		}

		// Rebind VFs in switchdev mode
		if err := n.rebindVFsInSwitchdevMode(ctx, device); err != nil {
			log.Error(err, "Failed to rebind VFs in switchdev mode", "device", currentDevName)
			return err
		}
	}

	// Restore PF MTU
	if err := n.setDeviceMTU(currentDevName, device.MTU); err != nil {
		log.Error(err, "Failed to set PF MTU", "device", currentDevName, "mtu", device.MTU)
		return err
	}

	// Restore representors if in switchdev mode
	if device.EswitchMode == eswitchModeSwitchdev && len(device.Representors) > 0 {
		if err := n.restoreRepresentors(ctx, currentDevName, device); err != nil {
			log.Error(err, "Failed to restore representors", "device", currentDevName)
			// Don't fail the entire restore for representor issues
		}
	}

	return nil
}

// getCurrentDeviceName gets the current device name after driver reload
func (n *netconfig) getCurrentDeviceName(pciAddr string) (string, error) {
	// Get device name from PCI path: /sys/bus/pci/devices/{pci_addr}/net/
	pciDevPath := fmt.Sprintf("%s%s/net", sysBusPCIDevicesPath, pciAddr)
	entries, err := n.os.ReadDir(pciDevPath)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no netdev found for PCI address %s", pciAddr)
	}

	return entries[0].Name(), nil
}

// setEswitchMode sets the eswitch mode for a device
func (n *netconfig) setEswitchMode(ctx context.Context, pciAddr, mode string) error {
	// Use devlink command: devlink dev eswitch set pci/{pci_addr} mode {mode}
	_, stderr, err := n.cmd.RunCommand(ctx, "devlink", "dev", "eswitch", "set", fmt.Sprintf("pci/%s", pciAddr), "mode", mode)
	if err != nil {
		return fmt.Errorf("failed to set eswitch mode to %s: %w, stderr: %s", mode, err, stderr)
	}
	return nil
}

// setDeviceAdminState sets the admin state of a device
func (n *netconfig) setDeviceAdminState(devName, state string) error {
	// Use netlink instead of ip command for better error handling and performance
	link, err := n.netlinkLib.LinkByName(devName)
	if err != nil {
		return fmt.Errorf("failed to get link %s: %w", devName, err)
	}

	if state == adminStateUp {
		err = n.netlinkLib.LinkSetUp(link)
	} else {
		err = n.netlinkLib.LinkSetDown(link)
	}

	if err != nil {
		return fmt.Errorf("failed to set device admin state to %s: %w", state, err)
	}
	return nil
}

// createVFs creates the specified number of VFs
func (n *netconfig) createVFs(pciAddr string, numVFs int) error {
	// Write to sriov_numvfs: echo {num_vfs} > /sys/bus/pci/devices/{pci_addr}/sriov_numvfs
	sriovNumVfsPath := fmt.Sprintf("%s%s/sriov_numvfs", sysBusPCIDevicesPath, pciAddr)
	numVFsStr := fmt.Sprintf("%d", numVFs)

	// Use the OS wrapper to write the file
	if err := n.os.WriteFile(sriovNumVfsPath, []byte(numVFsStr), 0o644); err != nil {
		return fmt.Errorf("failed to create %d VFs: %w", numVFs, err)
	}

	return nil
}

// restoreVFConfigurations restores the configuration for all VFs
func (n *netconfig) restoreVFConfigurations(ctx context.Context, devName string, device *MellanoxDevice, eswitchMode string) error {
	log := logr.FromContextOrDiscard(ctx)

	for _, vf := range device.VFs {
		log.V(1).Info("Restoring VF config", "device", devName, "vf_index", vf.VFIndex, "vf_pci", vf.VFPCIAddr)

		if err := n.restoreSingleVFConfig(ctx, devName, vf, device.DevType, eswitchMode); err != nil {
			log.Error(err, "Failed to restore VF config", "device", devName, "vf_index", vf.VFIndex)
			continue // Continue with other VFs
		}
	}

	return nil
}

// restoreSingleVFConfig restores the configuration for a single VF
func (n *netconfig) restoreSingleVFConfig(ctx context.Context, devName string, vf VF, devType string, eswitchMode string) error {
	log := logr.FromContextOrDiscard(ctx)

	// Restore VF-specific configuration based on device type
	if devType == devTypeIB {
		// For IB devices, set GUIDs
		if vf.GUID != "-" && vf.GUID != "" {
			if err := n.setIBGUIDs(ctx, devName, vf.VFIndex, vf.GUID); err != nil {
				log.Error(err, "Failed to set IB GUIDs", "device", devName, "vf_index", vf.VFIndex, "guid", vf.GUID)
				return err
			}
		}
	} else {
		// For Ethernet devices, set MAC addresses
		if err := n.setEthernetMACs(ctx, devName, vf); err != nil {
			log.Error(err, "Failed to set Ethernet MACs", "device", devName, "vf_index", vf.VFIndex)
			return err
		}
	}

	// Unbind VF from driver (always unbind, matches bash script)
	if err := n.unbindVFFromDriver(vf.VFPCIAddr); err != nil {
		log.Error(err, "Failed to unbind VF from driver", "device", devName, "vf_index", vf.VFIndex, "vf_pci", vf.VFPCIAddr)
		return err
	}

	// Rebind VF to driver (skip if in switchdev mode - handled separately)
	// This matches the bash script logic: if [ "${pf_eswitch_mode}" == "switchdev" ]; then continue; fi
	if eswitchMode != eswitchModeSwitchdev {
		if err := n.bindVFToDriver(vf.VFPCIAddr); err != nil {
			log.Error(err, "Failed to rebind VF to driver", "device", devName, "vf_index", vf.VFIndex, "vf_pci", vf.VFPCIAddr)
			return err
		}

		// Wait for bind delay (matches bash script)
		time.Sleep(3 * time.Second) // BIND_DELAY_SEC equivalent

		// Restore VF MTU and admin state after rebind
		if err := n.restoreVFState(vf); err != nil {
			log.Error(err, "Failed to restore VF state after rebind", "device", devName, "vf_index", vf.VFIndex, "vf_pci", vf.VFPCIAddr)
			return err
		}
	} else {
		log.V(1).Info("Skipping VF rebind for switchdev mode - will be handled after switchdev mode is set",
			"device", devName, "vf_index", vf.VFIndex)
	}

	return nil
}

// setIBGUIDs sets the GUIDs for an IB VF
func (n *netconfig) setIBGUIDs(ctx context.Context, devName string, vfIndex int, guid string) error {
	// Set port GUID: ip link set {dev_name} vf {vf_index} port_guid {guid}
	_, stderr, err := n.cmd.RunCommand(ctx, "ip", "link", "set", devName, "vf", fmt.Sprintf("%d", vfIndex), "port_guid", guid)
	if err != nil {
		return fmt.Errorf("failed to set port GUID: %w, stderr: %s", err, stderr)
	}

	// Set node GUID: ip link set {dev_name} vf {vf_index} node_guid {guid}
	_, stderr, err = n.cmd.RunCommand(ctx, "ip", "link", "set", devName, "vf", fmt.Sprintf("%d", vfIndex), "node_guid", guid)
	if err != nil {
		return fmt.Errorf("failed to set node GUID: %w, stderr: %s", err, stderr)
	}

	return nil
}

// setEthernetMACs sets the MAC addresses for an Ethernet VF
func (n *netconfig) setEthernetMACs(ctx context.Context, devName string, vf VF) error {
	// Get current VF device name
	currentVFName, err := n.getCurrentVFName(vf.VFPCIAddr)
	if err != nil {
		return fmt.Errorf("failed to get current VF name: %w", err)
	}

	// Set VF hardware MAC using netlink for better error handling
	link, err := n.netlinkLib.LinkByName(currentVFName)
	if err != nil {
		return fmt.Errorf("failed to get VF link %s: %w", currentVFName, err)
	}

	// Parse MAC address
	hwAddr, err := net.ParseMAC(vf.MACAddress)
	if err != nil {
		return fmt.Errorf("failed to parse VF MAC address %s: %w", vf.MACAddress, err)
	}

	if err := n.netlinkLib.LinkSetHardwareAddr(link, hwAddr); err != nil {
		return fmt.Errorf("failed to set VF hardware MAC: %w", err)
	}

	// Set VF admin MAC: ip link set dev {pf_name} vf {vf_index} mac {admin_mac}
	// Note: This still requires ip command as netlink doesn't have direct VF admin MAC support
	_, stderr, err := n.cmd.RunCommand(ctx, "ip", "link", "set", "dev", devName, "vf", fmt.Sprintf("%d", vf.VFIndex), "mac", vf.AdminMAC)
	if err != nil {
		return fmt.Errorf("failed to set VF admin MAC: %w, stderr: %s", err, stderr)
	}

	return nil
}

// getCurrentVFName gets the current VF device name after driver reload
func (n *netconfig) getCurrentVFName(vfPCIAddr string) (string, error) {
	// Get VF name from PCI path: /sys/bus/pci/devices/{vf_pci_addr}/net/
	vfPciDevPath := fmt.Sprintf("%s%s/net", sysBusPCIDevicesPath, vfPCIAddr)
	entries, err := n.os.ReadDir(vfPciDevPath)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no netdev found for VF PCI address %s", vfPCIAddr)
	}

	return entries[0].Name(), nil
}

// rebindVFsInSwitchdevMode rebinds VFs in switchdev mode
func (n *netconfig) rebindVFsInSwitchdevMode(ctx context.Context, device *MellanoxDevice) error {
	log := logr.FromContextOrDiscard(ctx)

	for _, vf := range device.VFs {
		log.V(1).Info("Rebinding VF in switchdev mode", "vf_pci", vf.VFPCIAddr)

		// Bind VF to driver
		if err := n.bindVFToDriver(vf.VFPCIAddr); err != nil {
			log.Error(err, "Failed to bind VF to driver", "vf_pci", vf.VFPCIAddr)
			continue
		}

		// Wait for bind delay (matches bash script)
		time.Sleep(3 * time.Second) // BIND_DELAY_SEC equivalent

		// Restore VF MTU and admin state
		if err := n.restoreVFState(vf); err != nil {
			log.Error(err, "Failed to restore VF state", "vf_pci", vf.VFPCIAddr)
			continue
		}
	}

	return nil
}

// getDriverPath gets the driver path for a VF PCI address
func (n *netconfig) getDriverPath(vfPCIAddr string) string {
	// Try to get the current driver from the VF's driver symlink
	driverLink := fmt.Sprintf("%s%s/driver", sysBusPCIDevicesPath, vfPCIAddr)
	driverPath, err := n.os.Readlink(driverLink)
	if err != nil {
		// If no driver is bound, use the default mlx5_core driver
		return defaultDriverPath
	}

	// Extract the driver name from the symlink path
	// driverPath is like "../../../../bus/pci/drivers/mlx5_core"
	parts := strings.Split(driverPath, "/")
	if len(parts) == 0 {
		return defaultDriverPath // Fallback to default
	}

	driverName := parts[len(parts)-1]
	return fmt.Sprintf("%s%s", sysBusPCIDriversPath, driverName)
}

// unbindVFFromDriver unbinds a VF from its driver
func (n *netconfig) unbindVFFromDriver(vfPCIAddr string) error {
	// Get the driver path for this VF
	driverPath := n.getDriverPath(vfPCIAddr)

	// Write VF PCI address to driver unbind file
	unbindFile := fmt.Sprintf("%s/unbind", driverPath)

	if err := n.os.WriteFile(unbindFile, []byte(vfPCIAddr), 0o644); err != nil {
		return fmt.Errorf("failed to unbind VF from driver: %w", err)
	}

	return nil
}

// bindVFToDriver binds a VF to its driver
func (n *netconfig) bindVFToDriver(vfPCIAddr string) error {
	// Get the driver path for this VF
	driverPath := n.getDriverPath(vfPCIAddr)

	// Write VF PCI address to driver bind file
	bindFile := fmt.Sprintf("%s/bind", driverPath)

	if err := n.os.WriteFile(bindFile, []byte(vfPCIAddr), 0o644); err != nil {
		return fmt.Errorf("failed to bind VF to driver: %w", err)
	}

	return nil
}

// restoreVFState restores the MTU and admin state of a VF
func (n *netconfig) restoreVFState(vf VF) error {
	// Get current VF name
	currentVFName, err := n.getCurrentVFName(vf.VFPCIAddr)
	if err != nil {
		return fmt.Errorf("failed to get current VF name: %w", err)
	}

	// Get VF link once and use it for both operations
	link, err := n.netlinkLib.LinkByName(currentVFName)
	if err != nil {
		return fmt.Errorf("failed to get VF link %s: %w", currentVFName, err)
	}

	// Set VF MTU using netlink
	if err := n.netlinkLib.LinkSetMTU(link, vf.MTU); err != nil {
		return fmt.Errorf("failed to set VF MTU to %d: %w", vf.MTU, err)
	}

	// Set VF admin state using netlink
	if vf.AdminState == adminStateUp {
		err = n.netlinkLib.LinkSetUp(link)
	} else {
		err = n.netlinkLib.LinkSetDown(link)
	}

	if err != nil {
		return fmt.Errorf("failed to set VF admin state to %s: %w", vf.AdminState, err)
	}

	return nil
}

// setDeviceMTU sets the MTU of a device
func (n *netconfig) setDeviceMTU(devName string, mtu int) error {
	// Use netlink instead of sysfs for better error handling and performance
	link, err := n.netlinkLib.LinkByName(devName)
	if err != nil {
		return fmt.Errorf("failed to get link %s: %w", devName, err)
	}

	if err := n.netlinkLib.LinkSetMTU(link, mtu); err != nil {
		return fmt.Errorf("failed to set device MTU to %d: %w", mtu, err)
	}

	return nil
}

// isMlx5CoreLoaded checks if the mlx5_core driver is loaded
func (n *netconfig) isMlx5CoreLoaded(ctx context.Context) (bool, error) {
	loadedModules, err := n.host.LsMod(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list loaded modules: %w", err)
	}

	// Check if mlx5_core is in the loaded modules map
	_, found := loadedModules["mlx5_core"]
	return found, nil
}

// discoverMellanoxDevices discovers all Mellanox network devices and collects detailed information
func (n *netconfig) discoverMellanoxDevices(ctx context.Context) ([]string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Get all network interfaces from sysfs (matches bash script approach)
	entries, err := n.os.ReadDir(sysClassNetPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read /sys/class/net: %w", err)
	}

	devices := make([]string, 0, len(entries))

	// Filter for Mellanox devices and collect detailed info
	for _, entry := range entries {
		devName := entry.Name()

		// Check vendor first (more efficient than PCI lookup)
		if !n.isMellanoxDeviceByInterface(devName) {
			continue
		}

		// Get PCI address using sriovnet library
		pciAddr, err := n.sriovnetLib.GetPciFromNetDevice(devName)
		if err != nil {
			log.V(1).Info("Could not get PCI address for device", "device", devName, "error", err)
			continue
		}

		log.V(1).Info("Found Mellanox device", "device", devName, "pci", pciAddr)

		// Get netlink link for additional attributes (admin state, MTU)
		link, err := n.netlinkLib.LinkByName(devName)
		if err != nil {
			log.V(1).Info("Could not get netlink link", "device", devName, "error", err)
			// Continue without netlink info - we can still collect basic info
			link = nil
		}
		// Get eswitch mode
		// This matches bash: eswitch_mode=$(devlink dev eswitch show pci/$pci_addr 2>/dev/null |
		// awk '{for (i=1; i<=NF; i++) if ($i == "mode") {print $(i+1); exit}}')
		eswitchMode, err := n.getEswitchMode(ctx, pciAddr)
		if err != nil {
			log.V(1).Info("Could not get eswitch mode", "device", devName, "pci", pciAddr, "error", err)
			eswitchMode = eswitchModeLegacy // Default to legacy mode
		}

		if eswitchMode == eswitchModeSwitchdev {
			// Skip VF representors
			if n.isRepresentor(devName) {
				log.V(1).Info("Skipping VF representor", "device", devName)
				continue
			}
		}

		// Collect detailed device information
		device := n.collectDeviceInfo(ctx, devName, pciAddr, link)

		device.EswitchMode = eswitchMode

		// Collect VF information if VFs are configured
		n.collectVFInfo(ctx, devName, device)

		// Store the device information
		n.mellanoxDevices[devName] = device
		devices = append(devices, devName)

		log.V(1).Info("Collected device info", "device", devName, "device", device, "vfs", len(device.VFs))
	}

	return devices, nil
}

// collectDeviceInfo collects detailed information about a Mellanox device
func (n *netconfig) collectDeviceInfo(ctx context.Context, devName, pciAddr string, link netlink.Link) *MellanoxDevice {
	log := logr.FromContextOrDiscard(ctx)

	device := &MellanoxDevice{
		PCIAddr: pciAddr,
		VFs:     make([]VF, 0), // Initialize empty VFs array
	}

	// Get admin state and MTU from netlink (preferred method)
	if link != nil {
		// Get admin state from netlink flags
		// This matches bash: dev_adminstate_flags=$(( $(cat "$netdev_path"/flags) & 1 ))
		// dev_adminstate=$([[ $dev_adminstate_flags -eq 1 ]] && echo "up" || echo "down")
		flags := link.Attrs().Flags
		if flags&net.FlagUp != 0 {
			device.AdminState = adminStateUp
		} else {
			device.AdminState = adminStateDown
		}

		// Get MTU from netlink attributes
		device.MTU = link.Attrs().MTU
	} else {
		// Fallback: read from sysfs directly (should be rare with netlink)
		log.V(1).Info("Netlink unavailable, falling back to sysfs", "device", devName)
		device.AdminState = n.getAdminStateFromSysfs(devName)
		device.MTU = n.getMTUFromSysfs(devName)
	}

	// Determine device type and get GUID
	// This matches bash: if [[ "$dev_name" =~ ^ib.* ]]; then dev_type="ib"; else dev_type="eth"; fi
	if strings.HasPrefix(devName, "ib") {
		device.DevType = devTypeIB
		// Get GUID for IB devices
		guid, err := n.getIBGUID(devName)
		if err != nil {
			log.V(1).Info("Could not get IB GUID", "device", devName, "error", err)
			device.GUID = "-"
		} else {
			device.GUID = n.restructureGUID(guid)
		}
	} else {
		device.DevType = devTypeEth
		device.GUID = "-"
	}

	// Get number of VFs from sysfs (matches bash script approach)
	device.PfNumVfs = n.getPfNumVfsFromSysfs(devName)

	return device
}

// collectVFInfo collects detailed information about VFs for a given PF
func (n *netconfig) collectVFInfo(ctx context.Context, devName string, device *MellanoxDevice) {
	log := logr.FromContextOrDiscard(ctx)

	// Skip if no VFs configured
	if device.PfNumVfs == 0 {
		return
	}

	log.V(1).Info("Collecting VF information", "device", devName, "vfs", device.PfNumVfs)

	// Collect VF information for each VF index
	for vfIndex := range device.PfNumVfs {
		vf, err := n.collectSingleVFInfo(ctx, devName, vfIndex, device.DevType)
		if err != nil {
			log.V(1).Info("Could not collect VF info", "device", devName, "vf_index", vfIndex, "error", err)
			continue // Continue with other VFs
		}

		device.VFs = append(device.VFs, *vf)
		log.V(1).Info("Collected VF info", "device", devName, "vf", vf)
	}
}

// collectSingleVFInfo collects information for a single VF
func (n *netconfig) collectSingleVFInfo(ctx context.Context, devName string, vfIndex int, devType string) (*VF, error) {
	log := logr.FromContextOrDiscard(ctx)

	// VF device path: /sys/class/net/{PF_NAME}/device/virtfn{N}/net/{VF_NAME}
	vfDevBasePath := fmt.Sprintf("%s%s/device/virtfn%d/net/", sysClassNetPath, devName, vfIndex)

	// Get VF name
	vfName, err := n.getVFName(vfDevBasePath)
	if err != nil {
		return nil, fmt.Errorf("could not get VF name: %w", err)
	}

	vfNetdevPath := vfDevBasePath + vfName

	// Get VF PCI address
	vfPCIAddr, err := n.getVFPCIAddr(vfNetdevPath)
	if err != nil {
		return nil, fmt.Errorf("could not get VF PCI address: %w", err)
	}

	// Get VF admin state, MAC address, and MTU using netlink (preferred method)
	vfAdminState, vfMAC, vfMTU, err := n.getVFAttributesFromNetlink(vfName)
	if err != nil {
		// Fallback to sysfs methods if netlink fails
		log.V(1).Info("Netlink failed for VF, falling back to sysfs", "vf", vfName, "error", err)

		vfAdminState, err = n.getVFAdminState(vfNetdevPath)
		if err != nil {
			return nil, fmt.Errorf("could not get VF admin state: %w", err)
		}

		vfMAC, err = n.getVFMACAddress(vfNetdevPath)
		if err != nil {
			return nil, fmt.Errorf("could not get VF MAC address: %w", err)
		}

		vfMTU, err = n.getVFMTU(vfNetdevPath)
		if err != nil {
			return nil, fmt.Errorf("could not get VF MTU: %w", err)
		}
	}

	// Get VF admin MAC and GUID using ip command (matches bash script approach)
	vfAdminMAC, vfGUID, err := n.getVFAdminMACAndGUID(ctx, devName, vfIndex, devType)
	if err != nil {
		log.V(1).Info("Could not get VF admin MAC/GUID", "device", devName, "vf_index", vfIndex, "error", err)
		// Use fallback values
		vfAdminMAC = vfMAC // Fallback to hardware MAC
		vfGUID = "-"       // Default for Ethernet
		if devType == devTypeIB {
			vfGUID = "" // Default for IB when extraction fails
		}
	}

	vf := &VF{
		VFIndex:    vfIndex,
		VFPCIAddr:  vfPCIAddr,
		VFName:     vfName,
		AdminState: vfAdminState,
		MACAddress: vfMAC,
		AdminMAC:   vfAdminMAC,
		MTU:        vfMTU,
		GUID:       vfGUID,
	}

	return vf, nil
}

// getVFAttributesFromNetlink gets VF admin state, MAC address, and MTU using netlink
func (n *netconfig) getVFAttributesFromNetlink(vfName string) (string, string, int, error) {
	link, err := n.netlinkLib.LinkByName(vfName)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to get VF link: %w", err)
	}

	attrs := link.Attrs()

	// Get admin state from netlink flags
	var adminState string
	if attrs.Flags&net.FlagUp != 0 {
		adminState = adminStateUp
	} else {
		adminState = adminStateDown
	}

	// Get MAC address from netlink attributes
	macAddr := attrs.HardwareAddr.String()
	if macAddr == "" {
		return "", "", 0, fmt.Errorf("no hardware address found")
	}

	// Get MTU from netlink attributes
	mtu := attrs.MTU
	if mtu == 0 {
		mtu = 1500 // Default MTU
	}

	return adminState, macAddr, mtu, nil
}

// getIBGUID gets the GUID for an InfiniBand device
func (n *netconfig) getIBGUID(devName string) (string, error) {
	// This matches bash: sysfs_guid=$(cat ${netdev_path}/device/infiniband/*/node_guid)
	// Look for the first infiniband directory under the device
	devicePath := fmt.Sprintf("%s%s/device/infiniband", sysClassNetPath, devName)

	// List infiniband directories
	entries, err := n.os.ReadDir(devicePath)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no infiniband directory found")
	}

	// Read the first node_guid file
	guidPath := fmt.Sprintf("%s/%s/node_guid", devicePath, entries[0].Name())
	guidData, err := n.os.ReadFile(guidPath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(guidData)), nil
}

// restructureGUID restructures the GUID format
func (n *netconfig) restructureGUID(guid string) string {
	// This matches the improved implementation
	// sysfs_guid is like "0c42a1030016054c"
	// restructure as "0c42:a103:0016:054c"
	raw := strings.TrimSpace(guid)
	if len(raw) != 16 {
		return guid // Return original if not expected format
	}
	return fmt.Sprintf("%s:%s:%s:%s", raw[0:4], raw[4:8], raw[8:12], raw[12:16])
}

// getEswitchMode gets the eswitch mode for a PCI device
func (n *netconfig) getEswitchMode(ctx context.Context, pciAddr string) (string, error) {
	// This matches bash: eswitch_mode=$(devlink dev eswitch show pci/$pci_addr 2>/dev/null |
	// awk '{for (i=1; i<=NF; i++) if ($i == "mode") {print $(i+1); exit}}')
	stdout, stderr, err := n.cmd.RunCommand(ctx, "devlink", "dev", "eswitch", "show", fmt.Sprintf("pci/%s", pciAddr))
	if err != nil {
		return "", fmt.Errorf("failed to run devlink command: %w, stderr: %s", err, stderr)
	}

	// Parse the output to find the mode
	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		for i, field := range fields {
			if field == "mode" && i+1 < len(fields) {
				return fields[i+1], nil
			}
		}
	}

	return "legacy", nil // Default to legacy if not found
}

// isMellanoxDeviceByInterface checks if a network interface is a Mellanox device by vendor
func (n *netconfig) isMellanoxDeviceByInterface(devName string) bool {
	// Read vendor ID from sysfs
	vendorPath := fmt.Sprintf("%s%s/device/vendor", sysClassNetPath, devName)
	vendorData, err := n.os.ReadFile(vendorPath)
	if err != nil {
		return false
	}

	// Mellanox vendor ID is 0x15b3
	return strings.TrimSpace(string(vendorData)) == "0x15b3"
}

// isRepresentor checks if a device is a VF representor
func (n *netconfig) isRepresentor(devName string) bool {
	// Read phys_port_name to check if it's a representor
	physPortNamePath := fmt.Sprintf("%s%s/phys_port_name", sysClassNetPath, devName)
	physPortNameData, err := n.os.ReadFile(physPortNamePath)
	if err != nil {
		return false
	}

	physPortName := strings.TrimSpace(string(physPortNameData))
	// Check if it's a representor: starts with "pf" and contains "vf"
	return strings.HasPrefix(physPortName, "pf") && strings.Contains(physPortName, "vf")
}

// getNetNamePath gets the udev-based network name path
func (n *netconfig) getNetNamePath(ctx context.Context, devName string) (string, error) {
	// This matches: udevadm info --query=property /sys/class/net/{iface}
	stdout, stderr, err := n.cmd.RunCommand(ctx, "udevadm", "info", "--query=property",
		fmt.Sprintf("%s%s", sysClassNetPath, devName))
	if err != nil {
		return "", fmt.Errorf("failed to run udevadm command: %w, stderr: %s", err, stderr)
	}

	// Parse the output to find ID_NET_NAME_PATH
	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ID_NET_NAME_PATH=") {
			return strings.TrimPrefix(line, "ID_NET_NAME_PATH="), nil
		}
	}

	return "", nil // Not found, return empty string
}

// getAdminStateFromSysfs gets the admin state from sysfs flags
func (n *netconfig) getAdminStateFromSysfs(devName string) string {
	// Read flags from sysfs: /sys/class/net/{dev}/flags
	flagsPath := fmt.Sprintf("%s%s/flags", sysClassNetPath, devName)
	flagsData, err := n.os.ReadFile(flagsPath)
	if err != nil {
		return adminStateDown // Default to down if we can't read
	}

	// Parse flags and check if bit 0 (IFF_UP) is set
	// This matches bash: dev_adminstate_flags=$(( $(cat "$netdev_path"/flags) & 1 ))
	flagsStr := strings.TrimSpace(string(flagsData))
	if flagsStr == "" {
		return adminStateDown
	}

	// Parse the flags value (handle both decimal and hexadecimal formats)
	var flags int
	if strings.HasPrefix(flagsStr, "0x") {
		// Parse hexadecimal format
		flags64, err := strconv.ParseInt(flagsStr, 0, 64)
		if err != nil {
			return adminStateDown
		}
		flags = int(flags64)
	} else {
		// Parse decimal format
		flags, err = strconv.Atoi(flagsStr)
		if err != nil {
			return adminStateDown
		}
	}

	// Check if bit 0 is set (IFF_UP flag)
	if flags&1 != 0 {
		return adminStateUp
	}

	return adminStateDown
}

// getMTUFromSysfs gets the MTU from sysfs
func (n *netconfig) getMTUFromSysfs(devName string) int {
	// Read MTU from sysfs: /sys/class/net/{dev}/mtu
	mtuPath := fmt.Sprintf("%s%s/mtu", sysClassNetPath, devName)
	mtuData, err := n.os.ReadFile(mtuPath)
	if err != nil {
		return 1500 // Default MTU if we can't read
	}

	// Parse MTU value
	mtuStr := strings.TrimSpace(string(mtuData))
	if mtuStr == "" {
		return 1500
	}

	// Convert to int
	mtu, err := strconv.Atoi(mtuStr)
	if err != nil {
		return 1500 // Default MTU if parsing fails
	}

	return mtu
}

// getPfNumVfsFromSysfs gets the number of VFs from sysfs
func (n *netconfig) getPfNumVfsFromSysfs(devName string) int {
	// Read sriov_numvfs from sysfs: /sys/class/net/{dev}/device/sriov_numvfs
	sriovNumVfsPath := fmt.Sprintf("%s%s/device/sriov_numvfs", sysClassNetPath, devName)
	sriovNumVfsData, err := n.os.ReadFile(sriovNumVfsPath)
	if err != nil {
		return 0 // Default to 0 if we can't read (device not SRIOV capable)
	}

	// Parse sriov_numvfs value
	sriovNumVfsStr := strings.TrimSpace(string(sriovNumVfsData))
	if sriovNumVfsStr == "" {
		return 0
	}

	// Convert to int
	sriovNumVfs, err := strconv.Atoi(sriovNumVfsStr)
	if err != nil {
		return 0 // Default to 0 if parsing fails
	}

	return sriovNumVfs
}

// getVFName gets the VF netdev name from the VF device base path
func (n *netconfig) getVFName(vfDevBasePath string) (string, error) {
	// List the net directory to get VF name (matches bash: vf_name=$(ls "$vf_dev_base_path"))
	entries, err := n.os.ReadDir(vfDevBasePath)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no VF netdev found in %s", vfDevBasePath)
	}

	return entries[0].Name(), nil
}

// getVFPCIAddr gets the VF PCI address from the VF netdev path
func (n *netconfig) getVFPCIAddr(vfNetdevPath string) (string, error) {
	// Read the device symlink and get basename (matches bash: vf_pci_addr=$(basename $(readlink "$vf_netdev_path"/device)))
	deviceLink := vfNetdevPath + "/device"
	linkTarget, err := n.os.Readlink(deviceLink)
	if err != nil {
		return "", err
	}

	// Get basename of the link target
	parts := strings.Split(linkTarget, "/")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid device link target: %s", linkTarget)
	}

	return parts[len(parts)-1], nil
}

// getVFAdminState gets the VF admin state from the VF netdev path
func (n *netconfig) getVFAdminState(vfNetdevPath string) (string, error) {
	// Read flags from sysfs (matches bash: vf_adminstate_flags=$(( $(cat "$vf_netdev_path"/flags) & 1 )))
	flagsPath := vfNetdevPath + "/flags"
	flagsData, err := n.os.ReadFile(flagsPath)
	if err != nil {
		return "", err
	}

	flagsStr := strings.TrimSpace(string(flagsData))
	if flagsStr == "" {
		return adminStateDown, nil
	}

	// Handle both decimal and hexadecimal formats
	var flags int
	if strings.HasPrefix(flagsStr, "0x") {
		// Parse hexadecimal format
		flags64, err := strconv.ParseInt(flagsStr, 0, 64)
		if err != nil {
			return adminStateDown, err
		}
		flags = int(flags64)
	} else {
		// Parse decimal format
		flags, err = strconv.Atoi(flagsStr)
		if err != nil {
			return adminStateDown, err
		}
	}

	// Check if bit 0 is set (IFF_UP flag)
	if flags&1 != 0 {
		return adminStateUp, nil
	}

	return adminStateDown, nil
}

// getVFMACAddress gets the VF MAC address from the VF netdev path
func (n *netconfig) getVFMACAddress(vfNetdevPath string) (string, error) {
	// Read MAC address from sysfs (matches bash: vf_mac=$(cat "$vf_netdev_path"/address))
	addressPath := vfNetdevPath + "/address"
	macData, err := n.os.ReadFile(addressPath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(macData)), nil
}

// getVFMTU gets the VF MTU from the VF netdev path
func (n *netconfig) getVFMTU(vfNetdevPath string) (int, error) {
	// Read MTU from sysfs (matches bash: vf_mtu_val=$(cat "$vf_netdev_path"/mtu))
	mtuPath := vfNetdevPath + "/mtu"
	mtuData, err := n.os.ReadFile(mtuPath)
	if err != nil {
		return 0, err
	}

	mtuStr := strings.TrimSpace(string(mtuData))
	if mtuStr == "" {
		return 1500, nil // Default MTU
	}

	mtu, err := strconv.Atoi(mtuStr)
	if err != nil {
		return 1500, err // Default MTU if parsing fails
	}

	return mtu, nil
}

// getVFAdminMACAndGUID gets VF admin MAC and GUID using ip command (matches bash script approach)
func (n *netconfig) getVFAdminMACAndGUID(ctx context.Context, devName string, vfIndex int, devType string) (string, string, error) {
	// Use ip command to get VF info (matches bash: vf_ip_link_json=$(ip -j link show $mlnx_dev_name | jq -r .[0].vfinfo_list[$vf_index]))
	stdout, stderr, err := n.cmd.RunCommand(ctx, "ip", "-j", "link", "show", devName)
	if err != nil {
		return "", "", fmt.Errorf("failed to run ip command: %w, stderr: %s", err, stderr)
	}

	// Parse JSON output to get VF info
	var linkInfos []LinkInfo
	if err := json.Unmarshal([]byte(stdout), &linkInfos); err != nil {
		return "", "", fmt.Errorf("failed to parse JSON output: %w", err)
	}

	if len(linkInfos) == 0 {
		return "", "", fmt.Errorf("no link info found for device %s", devName)
	}

	linkInfo := linkInfos[0]
	if len(linkInfo.VFinfoList) <= vfIndex {
		return "", "", fmt.Errorf("VF index %d not found in vfinfo_list for device %s", vfIndex, devName)
	}

	vfInfo := linkInfo.VFinfoList[vfIndex]

	// Extract admin MAC (matches bash: vf_admin_mac=$(echo ${vf_ip_link_json} | jq -r .address))
	adminMAC := vfInfo.Address

	// Extract GUID for IB devices (matches bash: vf_guid=$(echo ${vf_ip_link_json} | jq -r '."port guid"'))
	guid := "-" // Default for Ethernet
	if devType == devTypeIB {
		guid = vfInfo.PortGUID
	}

	return adminMAC, guid, nil
}

// discoverSwitchdevRepresentors discovers and stores switchdev representor information
func (n *netconfig) discoverSwitchdevRepresentors(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Discovering switchdev representors")

	representorCount := 0

	// Iterate through all discovered devices
	for devName, device := range n.mellanoxDevices {
		// Only process devices in switchdev mode
		if device.EswitchMode != eswitchModeSwitchdev {
			log.V(1).Info("Device not in switchdev mode, skipping representor discovery", "device", devName)
			continue
		}

		// Skip devices with no VFs
		if device.PfNumVfs == 0 {
			log.V(1).Info("Device has no VFs, skipping representor discovery", "device", devName)
			continue
		}

		log.Info("Discovering representors for device", "device", devName, "vfs", device.PfNumVfs)

		// Get physical port information
		physPortName, err := n.getPhysPortName(devName)
		if err != nil {
			log.Error(err, "Failed to get physical port name", "device", devName)
			return fmt.Errorf("failed to get physical port name for device %s: %w", devName, err)
		}

		physSwitchID, err := n.getPhysSwitchID(devName)
		if err != nil {
			log.Error(err, "Failed to get physical switch ID", "device", devName)
			return fmt.Errorf("failed to get physical switch ID for device %s: %w", devName, err)
		}

		// Parse physical port number from phys_port_name (format: "p1", "p2", etc.)
		physPortNum, err := n.parsePhysPortNumber(physPortName)
		if err != nil {
			log.Error(err, "Failed to parse physical port number", "device", devName, "phys_port_name", physPortName)
			return fmt.Errorf("failed to parse physical port number for device %s: %w", devName, err)
		}

		// Discover representors in the device's subsystem
		representors, err := n.findDeviceRepresentors(ctx, devName, physSwitchID, physPortNum)
		if err != nil {
			log.Error(err, "Failed to find representors for device", "device", devName)
			return fmt.Errorf("failed to find representors for device %s: %w", devName, err)
		}

		// Store representors in the device
		device.Representors = representors
		representorCount += len(representors)

		log.Info("Found representors for device", "device", devName, "count", len(representors))
	}

	log.Info("Switchdev representor discovery completed", "total_representors", representorCount)
	return nil
}

// getPhysPortName gets the physical port name for a device
func (n *netconfig) getPhysPortName(devName string) (string, error) {
	physPortPath := fmt.Sprintf("%s%s/phys_port_name", sysClassNetPath, devName)
	physPortName, err := n.os.ReadFile(physPortPath)
	if err != nil {
		return "", fmt.Errorf("failed to read phys_port_name: %w", err)
	}
	return strings.TrimSpace(string(physPortName)), nil
}

// getPhysSwitchID gets the physical switch ID for a device
func (n *netconfig) getPhysSwitchID(devName string) (string, error) {
	physSwitchPath := fmt.Sprintf("%s%s/phys_switch_id", sysClassNetPath, devName)
	physSwitchID, err := n.os.ReadFile(physSwitchPath)
	if err != nil {
		return "", fmt.Errorf("failed to read phys_switch_id: %w", err)
	}
	return strings.TrimSpace(string(physSwitchID)), nil
}

// parsePhysPortNumber parses the physical port number from phys_port_name
// Format: "p1", "p2", etc. -> returns "1", "2", etc.
func (n *netconfig) parsePhysPortNumber(physPortName string) (string, error) {
	if !strings.HasPrefix(physPortName, "p") {
		return "", fmt.Errorf("invalid phys_port_name format: %s", physPortName)
	}
	return strings.TrimPrefix(physPortName, "p"), nil
}

// findDeviceRepresentors finds representors for a specific device
func (n *netconfig) findDeviceRepresentors(ctx context.Context, devName, physSwitchID, physPortNum string) ([]Representor, error) {
	log := logr.FromContextOrDiscard(ctx)
	representors := make([]Representor, 0, 10) // Pre-allocate with capacity

	// Look for representors in the device's subsystem
	subsystemPath := fmt.Sprintf("%s%s/subsystem", sysClassNetPath, devName)
	entries, err := n.os.ReadDir(subsystemPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read subsystem directory: %w", err)
	}

	for _, entry := range entries {
		representorName := entry.Name()
		representorPath := fmt.Sprintf("%s%s/subsystem/%s", sysClassNetPath, devName, representorName)

		// Check if this is a representor by examining phys_port_name
		physPortNamePath := fmt.Sprintf("%s/phys_port_name", representorPath)
		physPortName, err := n.os.ReadFile(physPortNamePath)
		if err != nil {
			continue // Skip if we can't read phys_port_name
		}

		physPortNameStr := strings.TrimSpace(string(physPortName))

		// Check if this is a representor (format: "pf{port_num}vf{vf_id}")
		if !n.isRepresentorPhysPortName(physPortNameStr) {
			continue
		}

		// Parse representor information
		pfPortNum, vfID, err := n.parseRepresentorPhysPortName(physPortNameStr)
		if err != nil {
			log.V(1).Info("Failed to parse representor phys_port_name",
				"representor", representorName, "phys_port_name", physPortNameStr, "error", err)
			continue
		}

		// Verify this representor belongs to our PF
		if pfPortNum != physPortNum {
			log.V(1).Info("Representor does not belong to this PF",
				"representor", representorName, "pf_port", physPortNum, "representor_pf_port", pfPortNum)
			continue
		}

		// Verify physical switch ID matches
		representorSwitchID, err := n.getPhysSwitchID(representorName)
		if err != nil || representorSwitchID != physSwitchID {
			log.V(1).Info("Representor switch ID does not match PF",
				"representor", representorName, "pf_switch_id", physSwitchID, "representor_switch_id", representorSwitchID)
			continue
		}

		// Get representor configuration
		representor, err := n.collectRepresentorInfo(representorName, physSwitchID, physPortNum, vfID)
		if err != nil {
			log.Error(err, "Failed to collect representor info", "representor", representorName)
			continue
		}

		representors = append(representors, *representor)
		log.V(1).Info("Found representor", "name", representorName, "vf_id", vfID, "admin_state", representor.AdminState, "mtu", representor.MTU)
	}

	return representors, nil
}

// isRepresentorPhysPortName checks if a phys_port_name indicates a representor
func (n *netconfig) isRepresentorPhysPortName(physPortName string) bool {
	// Format: "pf{port_num}vf{vf_id}" (e.g., "pf1vf3")
	re := regexp.MustCompile(`^pf(\d+)vf(\d+)$`)
	return re.MatchString(physPortName)
}

// parseRepresentorPhysPortName parses representor phys_port_name to extract PF port and VF ID
func (n *netconfig) parseRepresentorPhysPortName(physPortName string) (pfPortNum, vfID string, err error) {
	re := regexp.MustCompile(`^pf(\d+)vf(\d+)$`)
	matches := re.FindStringSubmatch(physPortName)
	if len(matches) != 3 {
		return "", "", fmt.Errorf("invalid representor phys_port_name format: %s", physPortName)
	}
	return matches[1], matches[2], nil
}

// collectRepresentorInfo collects configuration information for a representor
func (n *netconfig) collectRepresentorInfo(representorName, physSwitchID, physPortNum, vfID string) (*Representor, error) {
	// Get admin state and MTU using netlink
	link, err := n.netlinkLib.LinkByName(representorName)
	if err != nil {
		return nil, fmt.Errorf("failed to get representor link: %w", err)
	}

	attrs := link.Attrs()

	// Get admin state from netlink flags
	var adminState string
	if attrs.Flags&net.FlagUp != 0 {
		adminState = adminStateUp
	} else {
		adminState = adminStateDown
	}

	// Get MTU from netlink attributes
	mtu := attrs.MTU

	return &Representor{
		PhysSwitchID: physSwitchID,
		PhysPortNum:  physPortNum,
		VFID:         vfID,
		Name:         representorName,
		AdminState:   adminState,
		MTU:          mtu,
	}, nil
}

// restoreRepresentors restores representor configurations
func (n *netconfig) restoreRepresentors(ctx context.Context, pfName string, device *MellanoxDevice) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("Restoring representors", "device", pfName, "count", len(device.Representors))

	// Get PF physical switch ID for matching
	pfPhysSwitchID, err := n.getPhysSwitchID(pfName)
	if err != nil {
		return fmt.Errorf("failed to get PF physical switch ID: %w", err)
	}

	// Get PF physical port number for matching
	pfPhysPortName, err := n.getPhysPortName(pfName)
	if err != nil {
		return fmt.Errorf("failed to get PF physical port name: %w", err)
	}

	pfPhysPortNum, err := n.parsePhysPortNumber(pfPhysPortName)
	if err != nil {
		return fmt.Errorf("failed to parse PF physical port number: %w", err)
	}

	// Restore each representor
	for _, representor := range device.Representors {
		log.Info("Restoring representor",
			"name", representor.Name, "vf_id", representor.VFID, "admin_state", representor.AdminState, "mtu", representor.MTU)

		// Find the current representor device
		currentRepresentorName, err := n.findCurrentRepresentor(ctx, pfPhysSwitchID, pfPhysPortNum, representor.VFID)
		if err != nil {
			log.Error(err, "Failed to find current representor", "original_name", representor.Name, "vf_id", representor.VFID)
			continue
		}

		// Rename representor if needed
		if currentRepresentorName != representor.Name {
			if err := n.renameRepresentor(ctx, currentRepresentorName, representor.Name); err != nil {
				log.Error(err, "Failed to rename representor", "current_name", currentRepresentorName, "target_name", representor.Name)
				continue
			}
			log.Info("Renamed representor", "from", currentRepresentorName, "to", representor.Name)
		}

		// Set representor MTU
		if err := n.setRepresentorMTU(representor.Name, representor.MTU); err != nil {
			log.Error(err, "Failed to set representor MTU", "representor", representor.Name, "mtu", representor.MTU)
			continue
		}

		// Set representor admin state
		if err := n.setRepresentorAdminState(representor.Name, representor.AdminState); err != nil {
			log.Error(err, "Failed to set representor admin state", "representor", representor.Name, "state", representor.AdminState)
			continue
		}

		log.Info("Successfully restored representor", "name", representor.Name)
	}

	log.Info("Representor restoration completed", "device", pfName)
	return nil
}

// findCurrentRepresentor finds the current representor device based on physical attributes
func (n *netconfig) findCurrentRepresentor(ctx context.Context, physSwitchID, physPortNum, vfID string) (string, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Scan all network devices to find the representor
	entries, err := n.os.ReadDir(sysClassNetPath)
	if err != nil {
		return "", fmt.Errorf("failed to read network devices: %w", err)
	}

	for _, entry := range entries {
		devName := entry.Name()

		// Skip the PF itself
		if devName == "lo" {
			continue
		}

		// Check if this device has the matching physical attributes
		devPhysSwitchID, err := n.getPhysSwitchID(devName)
		if err != nil {
			continue // Skip if we can't read phys_switch_id
		}

		if devPhysSwitchID != physSwitchID {
			continue // Different switch
		}

		// Check phys_port_name to see if it's a representor for our VF
		devPhysPortName, err := n.getPhysPortName(devName)
		if err != nil {
			continue // Skip if we can't read phys_port_name
		}

		// Check if this is a representor for our VF
		if !n.isRepresentorPhysPortName(devPhysPortName) {
			continue
		}

		// Parse representor information
		pfPortNum, devVFID, err := n.parseRepresentorPhysPortName(devPhysPortName)
		if err != nil {
			continue
		}

		// Check if this representor belongs to our PF and VF
		if pfPortNum == physPortNum && devVFID == vfID {
			log.V(1).Info("Found current representor", "name", devName, "vf_id", vfID)
			return devName, nil
		}
	}

	return "", fmt.Errorf("representor not found for VF ID %s", vfID)
}

// renameRepresentor renames a representor device
func (n *netconfig) renameRepresentor(ctx context.Context, currentName, newName string) error {
	// Use ip link set dev {current_name} name {new_name}
	_, stderr, err := n.cmd.RunCommand(ctx, "ip", "link", "set", "dev", currentName, "name", newName)
	if err != nil {
		return fmt.Errorf("failed to rename representor from %s to %s: %w, stderr: %s", currentName, newName, err, stderr)
	}
	return nil
}

// setRepresentorMTU sets the MTU for a representor
func (n *netconfig) setRepresentorMTU(representorName string, mtu int) error {
	// Use netlink for better error handling
	link, err := n.netlinkLib.LinkByName(representorName)
	if err != nil {
		return fmt.Errorf("failed to get representor link %s: %w", representorName, err)
	}

	if err := n.netlinkLib.LinkSetMTU(link, mtu); err != nil {
		return fmt.Errorf("failed to set representor MTU to %d: %w", mtu, err)
	}

	return nil
}

// setRepresentorAdminState sets the admin state for a representor
func (n *netconfig) setRepresentorAdminState(representorName, state string) error {
	// Use netlink for better error handling
	link, err := n.netlinkLib.LinkByName(representorName)
	if err != nil {
		return fmt.Errorf("failed to get representor link %s: %w", representorName, err)
	}

	if state == adminStateUp {
		err = n.netlinkLib.LinkSetUp(link)
	} else {
		err = n.netlinkLib.LinkSetDown(link)
	}

	if err != nil {
		return fmt.Errorf("failed to set representor admin state to %s: %w", state, err)
	}

	return nil
}

// DevicesUseNewNamingScheme returns true if interfaces with the new naming scheme are found.
func (n *netconfig) DevicesUseNewNamingScheme(ctx context.Context) (bool, error) {
	log := logr.FromContextOrDiscard(ctx)

	// Regex pattern to match np[0-3] suffix (new naming scheme)
	npPattern := regexp.MustCompile(`np[0-3]$`)

	// Get all network interfaces from sysfs (reuse existing logic)
	entries, err := n.os.ReadDir(sysClassNetPath)
	if err != nil {
		log.Error(err, "failed to list network devices")
		return false, err
	}

	// Check each network device
	for _, entry := range entries {
		devName := entry.Name()

		// Check if this is a NVIDIA device (reuse existing logic)
		if !n.isMellanoxDeviceByInterface(devName) {
			continue
		}

		log.V(1).Info("found NVIDIA device", "device", devName)

		// Use existing getNetNamePath function instead of duplicating udevadm logic
		netNamePath, err := n.getNetNamePath(ctx, devName)
		if err != nil {
			log.V(1).Info("failed to get NetNamePath for device", "device", devName, "error", err)
			continue
		}

		if netNamePath == "" {
			log.V(1).Info("no NetNamePath found for device", "device", devName)
			continue
		}

		log.V(1).Info("sampling interface for NetNamePath", "device", devName, "net_name_path", netNamePath)

		// Check if NetNamePath ends with np[0-3] pattern (new naming scheme)
		if npPattern.MatchString(netNamePath) {
			log.Info("device uses new naming scheme", "device", devName, "net_name_path", netNamePath)
			return true, nil
		}
	}

	log.Info("no devices found using new naming scheme")
	return false, nil
}
