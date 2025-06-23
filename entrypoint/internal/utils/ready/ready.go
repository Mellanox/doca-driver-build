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

package ready

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

// New initialize default implementation of the ready.Interface.
func New(path string, os wrappers.OSWrapper) Interface {
	return &ready{
		os:   os,
		path: path,
	}
}

// Interface is the interface exposed by the ready package.
type Interface interface {
	// Set creates the readiness indicator file.
	Set(ctx context.Context) error
	// Clear removes the readiness indicator file.
	Clear(ctx context.Context) error
}

type ready struct {
	os   wrappers.OSWrapper
	path string
}

// Set is the default implementation of the ready.Interface.
func (r *ready) Set(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("set driver ready indicator")
	// TODO add implementation
	return nil
}

// Clear is the default implementation of the ready.Interface.
func (r *ready) Clear(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("remove driver ready indicator")
	// TODO add implementation
	return nil
}
