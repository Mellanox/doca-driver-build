package netlink

import (
	"net"

	"github.com/vishvananda/netlink"
)

func New() Lib {
	return &libWrapper{}
}

type Link interface {
	netlink.Link
}

type Lib interface {
	// LinkByName finds a link by name and returns a pointer to the object.
	LinkByName(name string) (Link, error)
	// LinkSetUp enables the link device.
	// Equivalent to: `ip link set $link up`
	LinkSetUp(link Link) error
	// LinkSetDown disables the link device.
	// Equivalent to: `ip link set $link down`
	LinkSetDown(link Link) error
	// LinkSetMTU sets the mtu of the link device.
	// Equivalent to: `ip link set $link mtu $mtu`
	LinkSetMTU(link Link, mtu int) error
	// LinkSetHardwareAddr sets the hardware address of a link.
	LinkSetHardwareAddr(link Link, hwaddr net.HardwareAddr) error
	// GetLink returns the underlying netlink.Link from a Link interface
	GetLink(link Link) netlink.Link
}

type libWrapper struct{}

// LinkByName finds a link by name and returns a pointer to the object.
func (w *libWrapper) LinkByName(name string) (Link, error) {
	return netlink.LinkByName(name)
}

// LinkSetUp enables the link device.
// Equivalent to: `ip link set $link up`
func (w *libWrapper) LinkSetUp(link Link) error {
	return netlink.LinkSetUp(link)
}

// LinkSetDown disables the link device.
// Equivalent to: `ip link set $link down`
func (w *libWrapper) LinkSetDown(link Link) error {
	return netlink.LinkSetDown(link)
}

// LinkSetMTU sets the mtu of the link device.
// Equivalent to: `ip link set $link mtu $mtu`
func (w *libWrapper) LinkSetMTU(link Link, mtu int) error {
	return netlink.LinkSetMTU(link, mtu)
}

// LinkSetHardwareAddr sets the hardware address of a link.
func (w *libWrapper) LinkSetHardwareAddr(link Link, hwaddr net.HardwareAddr) error {
	return netlink.LinkSetHardwareAddr(link, hwaddr)
}

// GetLink returns the underlying netlink.Link from a Link interface
func (w *libWrapper) GetLink(link Link) netlink.Link {
	return link
}
