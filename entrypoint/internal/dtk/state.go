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

package dtk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// bootIDPath is overridable in tests.
var bootIDPath = "/proc/sys/kernel/random/boot_id"

// BootID returns the identifier of the current node boot. The DTK flags live in an emptyDir that
// survives a node reboot (containers restart in place), so the boot ID is used to tell a
// start flag written before the reboot from one written for the current attempt.
func BootID() (string, error) {
	return BootIDFrom(os.ReadFile)
}

// BootIDFrom is BootID with an injectable file reader, for callers that read host files
// through their own OS abstraction.
func BootIDFrom(readFile func(path string) ([]byte, error)) (string, error) {
	data, err := readFile(bootIDPath)
	if err != nil {
		return "", fmt.Errorf("failed to read boot id: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// HasBuiltRPMs reports whether the build tree contains at least one RPM under
// RPMS/redhat-release-*/<arch>/. install.pl can exit 0 with an empty RPMS directory
// when its source tree is removed mid-build, so the exit code alone is not proof of success.
func HasBuiltRPMs(srcDir string) bool {
	matches, err := filepath.Glob(filepath.Join(srcDir, "RPMS", "redhat-release-*", "*", "*.rpm"))
	return err == nil && len(matches) > 0
}

// SourceDirName returns the name of the driver source directory inside the shared DTK dir.
func SourceDirName(driverVersion string) string {
	return "MLNX_OFED_SRC-" + driverVersion
}
