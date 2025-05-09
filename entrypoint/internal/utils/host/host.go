/*
 Copyright 2024, NVIDIA CORPORATION & AFFILIATES

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

package host

import (
	"context"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

// New initialize default implementation of the host.Interface.
func New(c cmd.Interface, osWrapper wrappers.OSWrapper) Interface {
	return &host{
		cmd: c,
		os:  osWrapper,
	}
}

// Interface is the interface exposed by the host package.
type Interface interface {
	// GetOSType returns the name of the operating system as a string.
	GetOSType(ctx context.Context) (string, error)
	// GetDebugInfo returns a string containing debug information about the OS,
	// such as kernel version and memory info. This information is printed to the debug log.
	GetDebugInfo(ctx context.Context) (string, error)
	// LsMod returns list of the loaded kernel modules.
	LsMod(ctx context.Context) (map[string]LoadedModule, error)
	// RmMod unload the kernel module.
	RmMod(ctx context.Context, module string) error
}

type host struct {
	cmd cmd.Interface
	os  wrappers.OSWrapper
}

// GetOSType is the default implementation of the host.Interface.
func (h *host) GetOSType(ctx context.Context) (string, error) {
	// TODO: add implementation
	return "", nil
}

// GetDebugInfo is the default implementation of the host.Interface.
func (h *host) GetDebugInfo(ctx context.Context) (string, error) {
	// TODO: add implementation
	return "", nil
}

// LoadedModule contains information about loaded kernel module.
type LoadedModule struct {
	// Name of the kernel module.
	Name string
	// RefCount amount of refs to the module.
	RefCount int
	// UseBy contains names of the modules that depends on this module.
	UsedBy []string
}

// LsMod is the default implementation of the host.Interface.
func (h *host) LsMod(ctx context.Context) (map[string]LoadedModule, error) {
	// TODO: add implementation
	//nolint:nilnil
	return nil, nil
}

// RmMod is the default implementation of the host.Interface.
func (h *host) RmMod(ctx context.Context, module string) error {
	// TODO: add implementation
	return nil
}
