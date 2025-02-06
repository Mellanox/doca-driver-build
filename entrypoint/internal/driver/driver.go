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

package driver

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

// New creates a new instance of the driver manager
func New(containerMode string, cfg config.Config,
	c cmd.Interface, h host.Interface, osWrapper wrappers.OSWrapper,
) Interface {
	return &driverMgr{
		cfg:           cfg,
		containerMode: containerMode,
		cmd:           c,
		host:          h,
		os:            osWrapper,
	}
}

// Interface is the interface exposed by the driver package.
type Interface interface {
	// Prepare validates environment variables and performs required initialization for
	// the requested containerMode
	Prepare(ctx context.Context) error
	// Build installs required dependencies and build the driver
	Build(ctx context.Context) error
	// Load the new driver version. Returns a boolean indicating whether the driver was loaded successfully.
	// The function will return false if the system already has the same driver version loaded.
	Load(ctx context.Context) (bool, error)
	// Unload the driver and replace it with the inbox driver. Returns a boolean indicating whether the driver was unloaded successfully.
	// The function will return false if the system already runs with inbox driver.
	Unload(ctx context.Context) (bool, error)
	// Clear cleanups the system by removing unended leftovers.
	Clear(ctx context.Context) error
}

type driverMgr struct {
	cfg           config.Config
	containerMode string

	cmd  cmd.Interface
	host host.Interface
	os   wrappers.OSWrapper
}

// Prepare is the default implementation of the driver.Interface.
func (d *driverMgr) Prepare(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	switch d.containerMode {
	case constants.DriverContainerModeSources:
		log.Info("Executing driver sources container")
		if d.cfg.NvidiaNicDriverPath == "" {
			err := fmt.Errorf("NVIDIA_NIC_DRIVER_PATH environment variable must be set")
			log.Error(err, "missing required environment variable")
			return err
		}
		log.V(1).Info("Drivers source", "path", d.cfg.NvidiaNicDriverPath)
		if err := d.prepareGCC(ctx); err != nil {
			return err
		}
		if d.cfg.NvidiaNicDriversInventoryPath != "" {
			info, err := os.Stat(d.cfg.NvidiaNicDriversInventoryPath)
			if err != nil {
				log.Error(err, "path from NVIDIA_NIC_DRIVERS_INVENTORY_PATH environment variable is not accessible",
					"path", d.cfg.NvidiaNicDriversInventoryPath)
				return err
			}
			if !info.IsDir() {
				log.Error(err, "path from NVIDIA_NIC_DRIVERS_INVENTORY_PATH is not a dir",
					"path", d.cfg.NvidiaNicDriversInventoryPath)
				return fmt.Errorf("NVIDIA_NIC_DRIVERS_INVENTORY_PATH is not a dir")
			}
			log.V(1).Info("use driver inventory", "path", d.cfg.NvidiaNicDriversInventoryPath)
		}
		log.V(1).Info("driver inventory path is not set, container will always recompile driver on startup")
		return nil
	case constants.DriverContainerModePrecompiled:
		log.Info("Executing precompiled driver container")
		return nil
	default:
		return fmt.Errorf("unknown containerMode")
	}
}

// Build is the default implementation of the driver.Interface.
func (d *driverMgr) Build(ctx context.Context) error {
	// TODO: Implement
	return nil
}

// Load is the default implementation of the driver.Interface.
func (d *driverMgr) Load(ctx context.Context) (bool, error) {
	// TODO: Implement
	return true, nil
}

// Unload is the default implementation of the driver.Interface.
func (d *driverMgr) Unload(ctx context.Context) (bool, error) {
	// TODO: Implement
	return true, nil
}

// Clear is the default implementation of the driver.Interface.
func (d *driverMgr) Clear(ctx context.Context) error {
	// TODO: Implement
	return nil
}

func (d *driverMgr) prepareGCC(ctx context.Context) error {
	osType, err := d.host.GetOSType(ctx)
	if err != nil {
		return err
	}
	//nolint:gocritic
	switch osType {
	case "something":
	}
	return nil
}
