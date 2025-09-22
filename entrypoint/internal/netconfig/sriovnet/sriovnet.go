package sriovnet

import (
	"github.com/k8snetworkplumbingwg/sriovnet"
)

func New() Lib {
	return &libWrapper{}
}

type Lib interface {
	// GetPciFromNetDevice returns the PCI address associated with a network device name
	GetPciFromNetDevice(name string) (string, error)
}

type libWrapper struct{}

// GetPciFromNetDevice returns the PCI address associated with a network device name
func (w *libWrapper) GetPciFromNetDevice(name string) (string, error) {
	return sriovnet.GetPciFromNetDevice(name)
}
