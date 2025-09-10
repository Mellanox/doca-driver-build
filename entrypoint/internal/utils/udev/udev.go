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

package udev

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

// New initialize default implementation of the udev.Interface.
func New(path string, osWrapper wrappers.OSWrapper) Interface {
	return &udev{
		path: path,
		os:   osWrapper,
	}
}

// Interface is the interface exposed by the udev package.
type Interface interface {
	// CreateRules generates rules that preserve the old naming schema for NVIDIA interfaces.
	CreateRules(ctx context.Context) error
	// RemoveRules remove rules that preserve the old naming schema for NVIDIA interfaces.
	RemoveRules(ctx context.Context) error
	// DevicesUseNewNamingScheme returns true if interfaces with the new naming scheme
	// are on the host or if no NVIDIA devices are found.
	DevicesUseNewNamingScheme(ctx context.Context) (bool, error)
}

type udev struct {
	path string
	os   wrappers.OSWrapper
}

// CreateRules is the default implementation of the udev.Interface.
func (u *udev) CreateRules(ctx context.Context) error {
	_ = logr.FromContextOrDiscard(ctx)
	// TODO add implementation
	return nil
}

// RemoveRules is the default implementation of the udev.Interface.
func (u *udev) RemoveRules(ctx context.Context) error {
	_ = logr.FromContextOrDiscard(ctx)
	// TODO add implementation
	return nil
}

// DevicesUseNewNamingScheme is the default implementation of the udev.Interface
// The function scans the udev DB content directly.
func (u *udev) DevicesUseNewNamingScheme(ctx context.Context) (bool, error) {
	_ = logr.FromContextOrDiscard(ctx)
	// TODO add implementation
	return false, nil
}
