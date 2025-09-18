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

package udev

import (
	"context"
	"os"

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
	log := logr.FromContextOrDiscard(ctx)
	log.Info("create udev rules")

	// Create the udev rules content
	// This creates rules to rename physical interfaces by removing the "np[n]" suffix
	// Example: enp8s0f0np0 -> enp8s0f0, enp8s0f0np1v12 -> enp8s0f0v12
	udevRulesContent := `ACTION!="add", GOTO="mlnx_ofed_name_end"
SUBSYSTEM!="net", GOTO="mlnx_ofed_name_end"

# Rename physical interfaces (first case) of virtual functions (second case).
# Example names:
# enp8s0f0np0 -> enp8s0f0
# enp8s0f0np1v12 -> enp8s0f0v12

DRIVERS=="mlx5_core", ENV{ID_NET_NAME_PATH}!="", \
PROGRAM="/bin/sh -c 'echo $env{ID_NET_NAME_PATH} | sed -r -e s/np[01]$// -e s/np[01]v/v/'", \
        ENV{ID_NET_NAME_PATH}="$result"

DRIVERS=="mlx5_core", ENV{ID_NET_NAME_SLOT}!="", \
PROGRAM="/bin/sh -c 'echo $env{ID_NET_NAME_SLOT} | sed -r -e s/np[01]$// -e s/np[01]v/v/'", \
        ENV{ID_NET_NAME_SLOT}="$result"

LABEL="mlnx_ofed_name_end"
`

	// Write the udev rules file
	if err := u.os.WriteFile(u.path, []byte(udevRulesContent), 0o644); err != nil {
		log.Error(err, "failed to create udev rules file", "path", u.path)
		return err
	}

	log.Info("udev rules file created successfully", "path", u.path)

	// Read and log the file content on debug level (equivalent to bash: debug_print `cat ${MLX_UDEV_RULES_FILE}`)
	if content, err := u.os.ReadFile(u.path); err != nil {
		log.Error(err, "failed to read udev rules file for debug output", "path", u.path)
	} else {
		log.V(1).Info("udev rules file content", "path", u.path, "content", string(content))
	}

	return nil
}

// RemoveRules is the default implementation of the udev.Interface.
func (u *udev) RemoveRules(ctx context.Context) error {
	log := logr.FromContextOrDiscard(ctx)
	log.Info("remove udev rules")

	// Check if the udev rules file exists
	_, err := u.os.Stat(u.path)
	if err != nil {
		// Check if it's a "file not found" error
		if os.IsNotExist(err) {
			// File doesn't exist, log and skip
			log.Info("udev rules file was not previously created, skipping", "path", u.path)
			return nil
		}
		// Other errors (permission denied, etc.) should be returned
		log.Error(err, "failed to check if udev rules file exists", "path", u.path)
		return err
	}

	// File exists, delete it
	log.Info("deleting udev rules", "path", u.path)
	if err := u.os.RemoveAll(u.path); err != nil {
		log.Error(err, "failed to remove udev rules file", "path", u.path)
		return err
	}

	log.Info("udev rules file deleted successfully", "path", u.path)
	return nil
}

// DevicesUseNewNamingScheme is the default implementation of the udev.Interface
// The function scans the udev DB content directly.
func (u *udev) DevicesUseNewNamingScheme(ctx context.Context) (bool, error) {
	_ = logr.FromContextOrDiscard(ctx)
	// TODO add implementation
	return false, nil
}
