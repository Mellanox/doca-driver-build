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

//nolint:lll
package config

import (
	"github.com/caarlos0/env/v11"
)

// Config contains configuration for the entrypoint.
type Config struct {
	// public API
	UnloadStorageModules          bool `env:"UNLOAD_STORAGE_MODULES"`
	CreateIfnamesUdev             bool `env:"CREATE_IFNAMES_UDEV"`
	EnableNfsRdma                 bool `env:"ENABLE_NFSRDMA"`
	RestoreDriverOnPodTermination bool `env:"RESTORE_DRIVER_ON_POD_TERMINATION" envDefault:"true"`

	// driver manager advanced settings
	DriverReadyPath  string `env:"DRIVER_READY_PATH"   envDefault:"/run/mellanox/drivers/.driver-ready"`
	MlxUdevRulesFile string `env:"MLX_UDEV_RULES_FILE" envDefault:"/host/etc/udev/rules.d/77-mlnx-net-names.rules"`
	LockFilePath     string `env:"LOCK_FILE_PATH"      envDefault:"/run/mellanox/drivers/.lock"`

	NvidiaNicDriverVer    string `env:"NVIDIA_NIC_DRIVER_VER,required,notEmpty"`
	NvidiaNicDriverPath   string `env:"NVIDIA_NIC_DRIVER_PATH"`
	NvidiaNicContainerVer string `env:"NVIDIA_NIC_CONTAINER_VER"`

	DtkOcpDriverBuild             bool   `env:"DTK_OCP_DRIVER_BUILD"`
	DtkOcpNicSharedDir            string `env:"DTK_OCP_NIC_SHARED_DIR"            envDefault:"/mnt/shared-nvidia-nic-driver-toolkit"`
	NvidiaNicDriversInventoryPath string `env:"NVIDIA_NIC_DRIVERS_INVENTORY_PATH"`

	OfedBlacklistModulesFile string   `env:"OFED_BLACKLIST_MODULES_FILE" envDefault:"/etc/modprobe.d/blacklist-ofed-modules.conf"`
	OfedBlacklistModules     []string `env:"OFED_BLACKLIST_MODULES"      envDefault:"mlx5_core:mlx5_ib:ib_umad:ib_uverbs:ib_ipoib:rdma_cm:rdma_ucm:ib_core:ib_cm" envSeparator:":"`
	StorageModules           []string `env:"STORAGE_MODULES"             envDefault:"ib_isert:nvme_rdma:nvmet_rdma:rpcrdma:xprtrdma:ib_srpt"                      envSeparator:":"`

	// debug settings
	EntrypointDebug     bool   `env:"ENTRYPOINT_DEBUG"`
	DebugLogFile        string `env:"DEBUG_LOG_FILE"          envDefault:"/tmp/entrypoint_debug_cmds.log"`
	DebugSleepSecOnExit int    `env:"DEBUG_SLEEP_SEC_ON_EXIT" envDefault:"300"`
}

// GetConfig parses environment variables and returns a Config struct.
func GetConfig() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
