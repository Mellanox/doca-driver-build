/*
 Copyright 2026, NVIDIA CORPORATION & AFFILIATES

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

// PublishFlag atomically replaces path with data. Readers see either the previous
// contents or the complete new contents, never a truncated file. The temporary file
// is created in the same directory so the rename stays on one filesystem.
func PublishFlag(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-")
	if err != nil {
		return fmt.Errorf("failed to create temporary flag: %w", err)
	}
	tmp := f.Name()
	published := false
	defer func() {
		if !published {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o644); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to set flag permissions: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write temporary flag: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close temporary flag: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to publish flag %s: %w", path, err)
	}
	published = true
	return nil
}

// bootIDPath is overridable in tests.
var bootIDPath = "/proc/sys/kernel/random/boot_id"

// BootIDFrom returns the identifier of the current node boot. The DTK flags live in an emptyDir
// that survives a node reboot (containers restart in place), so the boot ID tells a start flag
// written before the reboot from one written for the current attempt. Callers pass their own
// file reader so the driver can read it through its OS wrapper.
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
