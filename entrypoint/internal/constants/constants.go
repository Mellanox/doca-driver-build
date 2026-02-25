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

package constants

const (
	MlxDriverName = "mlx5_core"

	DriverContainerModeSources     = "sources"
	DriverContainerModePrecompiled = "precompiled"
	DriverContainerModeDtkBuild    = "dtk-build"

	// OS Types
	OSTypeUbuntu    = "ubuntu"
	OSTypeSLES      = "sles"
	OSTypeRedHat    = "redhat"
	OSTypeOpenShift = "openshift"

	// Default versions
	DefaultRHELVersion      = "8.4"
	DefaultOpenShiftVersion = "4.9"

	InvalidGUID = "00:00:00:00:00:00:00:00"

	// DTK constants
	DtkOcpBuildScriptPath    = "/root/dtk_nic_driver_build.sh"
	DtkStartCompileFlag      = "dtk_start_compile"
	DtkDoneCompileFlagPrefix = "dtk_done_compile_"
)
