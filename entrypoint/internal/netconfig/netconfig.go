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

package netconfig

import "context"

// New initialize default implementation of the netconfig.Interface.
func New() Interface {
	return &netconfig{}
}

// Interface is the interface exposed by the netconfig package.
type Interface interface {
	// Save function preserves the current NVIDIA network configuration,
	// allowing it to be restored after a driver reload.
	// It supports PF, VF, and VF representor configurations.
	Save(ctx context.Context) error
	// Restore the saved configuration for NVIDIA devices.
	Restore(ctx context.Context) error
}

type netconfig struct{}

// Save is the default implementation of the netconfig.Interface.
func (n *netconfig) Save(ctx context.Context) error {
	return nil
}

// Restore is the default implementation of the netconfig.Interface.
func (n *netconfig) Restore(ctx context.Context) error {
	return nil
}
