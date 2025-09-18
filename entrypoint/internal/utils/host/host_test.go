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

package host

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	cmd_mocks "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
 	wrappers_mocks "github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers/mocks"
)

var _ = Describe("Host", func() {
	var (
		cmdMock *cmd_mocks.Interface
		osMock  *wrappers_mocks.OSWrapper
		h       Interface
		ctx     context.Context
	)
	
	BeforeEach(func() {
		cmdMock = cmd_mocks.NewInterface(GinkgoT())
		osMock = wrappers_mocks.NewOSWrapper(GinkgoT())
		h = New(cmdMock, osMock)
		ctx = context.Background()
	})

	Context("GetDebugInfo", func() {
		It("should return debug info with all successful operations", func() {
			osReleaseContent := `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
ID=ubuntu`
			unameOutput := "Linux hostname 5.15.0-91-generic #101-Ubuntu SMP Thu Nov 16 18:13:39 UTC 2023 x86_64 x86_64 x86_64 GNU/Linux"
			freeOutput := `              total        used        free      shared  buff/cache   available
Mem:           15947        1234        5678         123        8901       14000
Swap:          2048           0        2048`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(osReleaseContent), nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "uname", "-a").Return(unameOutput, "", nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "free", "-m").Return(freeOutput, "", nil)

			debugInfo, err := h.GetDebugInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(debugInfo).To(ContainSubstring("[os-release]: " + osReleaseContent))
			Expect(debugInfo).To(ContainSubstring("[uname -a]: " + unameOutput))
			Expect(debugInfo).To(ContainSubstring("[free -m]: " + freeOutput))
		})

		It("should handle os-release read error gracefully", func() {
			unameOutput := "Linux hostname 5.15.0-91-generic #101-Ubuntu SMP Thu Nov 16 18:13:39 UTC 2023 x86_64 x86_64 x86_64 GNU/Linux"
			freeOutput := `              total        used        free      shared  buff/cache   available
Mem:           15947        1234        5678         123        8901       14000
Swap:          2048           0        2048`

			osMock.EXPECT().ReadFile("/etc/os-release").Return(nil, assert.AnError)
			cmdMock.EXPECT().RunCommand(context.Background(), "uname", "-a").Return(unameOutput, "", nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "free", "-m").Return(freeOutput, "", nil)

			debugInfo, err := h.GetDebugInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(debugInfo).To(ContainSubstring("[os-release]: Error reading /etc/os-release: assert.AnError general error for testing"))
			Expect(debugInfo).To(ContainSubstring("[uname -a]: " + unameOutput))
			Expect(debugInfo).To(ContainSubstring("[free -m]: " + freeOutput))
		})

		It("should handle uname command error gracefully", func() {
			osReleaseContent := `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
ID=ubuntu`
			freeOutput := `              total        used        free      shared  buff/cache   available
Mem:           15947        1234        5678         123        8901       14000
Swap:          2048           0        2048`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(osReleaseContent), nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "uname", "-a").Return("", "command not found", assert.AnError)
			cmdMock.EXPECT().RunCommand(context.Background(), "free", "-m").Return(freeOutput, "", nil)

			debugInfo, err := h.GetDebugInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(debugInfo).To(ContainSubstring("[os-release]: " + osReleaseContent))
			Expect(debugInfo).To(ContainSubstring("[uname -a]: Error executing uname -a: assert.AnError general error for testing (stderr: command not found)"))
			Expect(debugInfo).To(ContainSubstring("[free -m]: " + freeOutput))
		})

		It("should handle free command error gracefully", func() {
			osReleaseContent := `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
ID=ubuntu`
			unameOutput := "Linux hostname 5.15.0-91-generic #101-Ubuntu SMP Thu Nov 16 18:13:39 UTC 2023 x86_64 x86_64 x86_64 GNU/Linux"

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(osReleaseContent), nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "uname", "-a").Return(unameOutput, "", nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "free", "-m").Return("", "permission denied", assert.AnError)

			debugInfo, err := h.GetDebugInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(debugInfo).To(ContainSubstring("[os-release]: " + osReleaseContent))
			Expect(debugInfo).To(ContainSubstring("[uname -a]: " + unameOutput))
			Expect(debugInfo).To(ContainSubstring("[free -m]: Error executing free -m: assert.AnError general error for testing (stderr: permission denied)"))
		})

		It("should handle all operations failing gracefully", func() {
			osMock.EXPECT().ReadFile("/etc/os-release").Return(nil, assert.AnError)
			cmdMock.EXPECT().RunCommand(context.Background(), "uname", "-a").Return("", "command not found", assert.AnError)
			cmdMock.EXPECT().RunCommand(context.Background(), "free", "-m").Return("", "permission denied", assert.AnError)

			debugInfo, err := h.GetDebugInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(debugInfo).To(ContainSubstring("[os-release]: Error reading /etc/os-release: assert.AnError general error for testing"))
			Expect(debugInfo).To(ContainSubstring("[uname -a]: Error executing uname -a: assert.AnError general error for testing (stderr: command not found)"))
			Expect(debugInfo).To(ContainSubstring("[free -m]: Error executing free -m: assert.AnError general error for testing (stderr: permission denied)"))
		})

		It("should handle empty outputs from commands", func() {
			osReleaseContent := `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
ID=ubuntu`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(osReleaseContent), nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "uname", "-a").Return("", "", nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "free", "-m").Return("", "", nil)

			debugInfo, err := h.GetDebugInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(debugInfo).To(ContainSubstring("[os-release]: " + osReleaseContent))
			Expect(debugInfo).To(ContainSubstring("[uname -a]: \n"))
			Expect(debugInfo).To(ContainSubstring("[free -m]: \n"))
		})

		It("should handle multiline os-release content", func() {
			osReleaseContent := `PRETTY_NAME="Red Hat Enterprise Linux 9.2 (Plow)"
NAME="Red Hat Enterprise Linux"
VERSION="9.2 (Plow)"
ID="rhel"
ID_LIKE="fedora"
VERSION_ID="9.2"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux 9.2 (Plow)"
ANSI_COLOR="0;31"
LOGO="fedora-logo-icon"
CPE_NAME="cpe:/o:redhat:enterprise_linux:9::baseos"
HOME_URL="https://www.redhat.com/"
DOCUMENTATION_URL="https://access.redhat.com/documentation/red_hat_enterprise_linux/9/"
SUPPORT_URL="https://access.redhat.com/support"
BUG_REPORT_URL="https://bugzilla.redhat.com/"
REDHAT_BUGZILLA_PRODUCT="Red Hat Enterprise Linux 9"
REDHAT_BUGZILLA_PRODUCT_VERSION=9.2
REDHAT_SUPPORT_PRODUCT="Red Hat Enterprise Linux"
REDHAT_SUPPORT_PRODUCT_VERSION="9.2"`
			unameOutput := "Linux rhel-host 5.14.0-284.11.1.el9_2.x86_64 #1 SMP PREEMPT_DYNAMIC Tue May 9 10:23:07 UTC 2023 x86_64 x86_64 x86_64 GNU/Linux"
			freeOutput := `              total        used        free      shared  buff/cache   available
Mem:           32000        8000       12000         500       12000       23000
Swap:          4096           0        4096`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(osReleaseContent), nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "uname", "-a").Return(unameOutput, "", nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "free", "-m").Return(freeOutput, "", nil)

			debugInfo, err := h.GetDebugInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(debugInfo).To(ContainSubstring("[os-release]: " + osReleaseContent))
			Expect(debugInfo).To(ContainSubstring("[uname -a]: " + unameOutput))
			Expect(debugInfo).To(ContainSubstring("[free -m]: " + freeOutput))
		})

		It("should handle commands with stderr output but no error", func() {
			osReleaseContent := `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
ID=ubuntu`
			unameOutput := "Linux hostname 5.15.0-91-generic #101-Ubuntu SMP Thu Nov 16 18:13:39 UTC 2023 x86_64 x86_64 x86_64 GNU/Linux"
			unameStderr := "warning: some warning message"
			freeOutput := `              total        used        free      shared  buff/cache   available
Mem:           15947        1234        5678         123        8901       14000
Swap:          2048           0        2048`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(osReleaseContent), nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "uname", "-a").Return(unameOutput, unameStderr, nil)
			cmdMock.EXPECT().RunCommand(context.Background(), "free", "-m").Return(freeOutput, "", nil)

			debugInfo, err := h.GetDebugInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(debugInfo).To(ContainSubstring("[os-release]: " + osReleaseContent))
			Expect(debugInfo).To(ContainSubstring("[uname -a]: " + unameOutput))
			Expect(debugInfo).To(ContainSubstring("[free -m]: " + freeOutput))
		})
	})
	Context("LsMod", func() {
		Context("when lsmod command succeeds", func() {
			It("should parse standard lsmod output correctly", func() {
				// Sample lsmod output
				lsmodOutput := `Module                  Size  Used by
mlx5_core             1234567  2 mlx5_ib,mlx5_netdev
mlx5_ib               987654   1
nvidia_peermem         45678   0
ib_core               234567   3 mlx5_ib,ib_isert,ib_srpt
ib_isert              123456   1
nvme_rdma              78901   2 nvme,ib_core
rpcrdma                34567   1
xprtrdma               23456   1
ib_srpt                12345   1
nvmet_rdma             67890   1`

				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return(lsmodOutput, "", nil)

				result, err := h.LsMod(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(10))

				// Test mlx5_core module
				mlx5Core, exists := result["mlx5_core"]
				Expect(exists).To(BeTrue())
				Expect(mlx5Core.Name).To(Equal("mlx5_core"))
				Expect(mlx5Core.RefCount).To(Equal(2))
				Expect(mlx5Core.UsedBy).To(Equal([]string{"mlx5_ib", "mlx5_netdev"}))

				// Test mlx5_ib module (no dependencies)
				mlx5Ib, exists := result["mlx5_ib"]
				Expect(exists).To(BeTrue())
				Expect(mlx5Ib.Name).To(Equal("mlx5_ib"))
				Expect(mlx5Ib.RefCount).To(Equal(1))
				Expect(mlx5Ib.UsedBy).To(BeEmpty())

				// Test nvidia_peermem module (unused)
				nvidiaPeermem, exists := result["nvidia_peermem"]
				Expect(exists).To(BeTrue())
				Expect(nvidiaPeermem.Name).To(Equal("nvidia_peermem"))
				Expect(nvidiaPeermem.RefCount).To(Equal(0))
				Expect(nvidiaPeermem.UsedBy).To(BeEmpty())

				// Test ib_core module (multiple dependencies)
				ibCore, exists := result["ib_core"]
				Expect(exists).To(BeTrue())
				Expect(ibCore.Name).To(Equal("ib_core"))
				Expect(ibCore.RefCount).To(Equal(3))
				Expect(ibCore.UsedBy).To(Equal([]string{"mlx5_ib", "ib_isert", "ib_srpt"}))
			})

			It("should handle empty lsmod output", func() {
				lsmodOutput := `Module                  Size  Used by`

				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return(lsmodOutput, "", nil)

				result, err := h.LsMod(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeEmpty())
			})

			It("should handle modules with no dependencies (represented by '-')", func() {
				lsmodOutput := `Module                  Size  Used by
standalone_module       12345   0 -
another_module          67890   1 -`

				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return(lsmodOutput, "", nil)

				result, err := h.LsMod(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(2))

				standalone, exists := result["standalone_module"]
				Expect(exists).To(BeTrue())
				Expect(standalone.UsedBy).To(BeEmpty())

				another, exists := result["another_module"]
				Expect(exists).To(BeTrue())
				Expect(another.UsedBy).To(BeEmpty())
			})

			It("should skip malformed lines gracefully", func() {
				lsmodOutput := `Module                  Size  Used by
valid_module            12345   1 dependency
malformed_line
another_valid_module    67890   2 dep1,dep2
incomplete_line         11111
final_valid_module      99999   0`

				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return(lsmodOutput, "", nil)

				result, err := h.LsMod(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(3))

				// Check that only valid modules are parsed
				Expect(result).To(HaveKey("valid_module"))
				Expect(result).To(HaveKey("another_valid_module"))
				Expect(result).To(HaveKey("final_valid_module"))
				Expect(result).NotTo(HaveKey("malformed_line"))
				Expect(result).NotTo(HaveKey("incomplete_line"))
			})

			It("should handle modules with complex dependency names", func() {
				lsmodOutput := `Module                  Size  Used by
complex_module          12345   2 module-with-dashes,module_with_underscores,module.with.dots`

				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return(lsmodOutput, "", nil)

				result, err := h.LsMod(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(1))

				complex, exists := result["complex_module"]
				Expect(exists).To(BeTrue())
				Expect(complex.UsedBy).To(Equal([]string{"module-with-dashes", "module_with_underscores", "module.with.dots"}))
			})

			It("should handle reference count parsing errors gracefully", func() {
				lsmodOutput := `Module                  Size  Used by
invalid_ref_count       12345   invalid dependency`

				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return(lsmodOutput, "", nil)

				result, err := h.LsMod(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(1))

				module, exists := result["invalid_ref_count"]
				Expect(exists).To(BeTrue())
				Expect(module.RefCount).To(Equal(0)) // Should default to 0 on parse error
			})
		})

		Context("when lsmod command fails", func() {
			It("should return error when command execution fails", func() {
				expectedError := errors.New("command not found")
				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return("", "lsmod: command not found", expectedError)

				result, err := h.LsMod(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to execute lsmod command"))
				Expect(err.Error()).To(ContainSubstring("lsmod: command not found"))
				Expect(result).To(BeNil())
			})

			It("should return error with stderr information", func() {
				expectedError := errors.New("permission denied")
				stderr := "lsmod: permission denied"
				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return("", stderr, expectedError)

				result, err := h.LsMod(ctx)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("stderr: lsmod: permission denied"))
				Expect(result).To(BeNil())
			})
		})

		Context("edge cases", func() {
			It("should handle whitespace-only lines", func() {
				lsmodOutput := `Module                  Size  Used by
valid_module            12345   1 dependency

another_valid_module    67890   0`

				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return(lsmodOutput, "", nil)

				result, err := h.LsMod(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(2))
				Expect(result).To(HaveKey("valid_module"))
				Expect(result).To(HaveKey("another_valid_module"))
			})

			It("should handle modules with spaces in dependency names", func() {
				lsmodOutput := `Module                  Size  Used by
test_module             12345   1 "module with spaces"`

				cmdMock.EXPECT().RunCommand(ctx, "lsmod").Return(lsmodOutput, "", nil)

				result, err := h.LsMod(ctx)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(1))

				module, exists := result["test_module"]
				Expect(exists).To(BeTrue())
				Expect(module.UsedBy).To(Equal([]string{`"module with spaces"`}))
			})
		})
	})

	Context("RmMod", func() {
		Context("when rmmod command succeeds", func() {
			It("should successfully unload a kernel module", func() {
				moduleName := "nvidia_peermem"
				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", "", nil)

				err := h.RmMod(ctx, moduleName)

				Expect(err).NotTo(HaveOccurred())
			})

			It("should successfully unload different kernel modules", func() {
				testCases := []string{
					"mlx5_core",
					"mlx5_ib",
					"ib_core",
					"nvidia_peermem",
					"test_module",
				}

				for _, moduleName := range testCases {
					cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", "", nil)

					err := h.RmMod(ctx, moduleName)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("should handle modules with special characters in names", func() {
				specialModules := []string{
					"module-with-dashes",
					"module_with_underscores",
					"module.with.dots",
					"module123",
				}

				for _, moduleName := range specialModules {
					cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", "", nil)

					err := h.RmMod(ctx, moduleName)
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})

		Context("when rmmod command fails", func() {
			It("should return error when module is not loaded", func() {
				moduleName := "nonexistent_module"
				expectedError := errors.New("module not found")
				stderr := "rmmod: ERROR: Module nonexistent_module is not currently loaded"

				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", stderr, expectedError)

				err := h.RmMod(ctx, moduleName)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unload kernel module nonexistent_module"))
				Expect(err.Error()).To(ContainSubstring("stderr: rmmod: ERROR: Module nonexistent_module is not currently loaded"))
			})

			It("should return error when module is in use", func() {
				moduleName := "module_in_use"
				expectedError := errors.New("module in use")
				stderr := "rmmod: ERROR: Module module_in_use is in use"

				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", stderr, expectedError)

				err := h.RmMod(ctx, moduleName)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unload kernel module module_in_use"))
				Expect(err.Error()).To(ContainSubstring("stderr: rmmod: ERROR: Module module_in_use is in use"))
			})

			It("should return error when permission is denied", func() {
				moduleName := "privileged_module"
				expectedError := errors.New("permission denied")
				stderr := "rmmod: ERROR: could not remove module privileged_module: Operation not permitted"

				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", stderr, expectedError)

				err := h.RmMod(ctx, moduleName)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unload kernel module privileged_module"))
				Expect(err.Error()).To(ContainSubstring("stderr: rmmod: ERROR: could not remove module privileged_module: Operation not permitted"))
			})

			It("should return error when rmmod command is not found", func() {
				moduleName := "test_module"
				expectedError := errors.New("command not found")
				stderr := "rmmod: command not found"

				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", stderr, expectedError)

				err := h.RmMod(ctx, moduleName)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unload kernel module test_module"))
				Expect(err.Error()).To(ContainSubstring("stderr: rmmod: command not found"))
			})

			It("should return error with empty stderr", func() {
				moduleName := "test_module"
				expectedError := errors.New("unknown error")

				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", "", expectedError)

				err := h.RmMod(ctx, moduleName)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unload kernel module test_module"))
				Expect(err.Error()).To(ContainSubstring("stderr: "))
			})
		})

		Context("edge cases", func() {
			It("should handle empty module name", func() {
				moduleName := ""
				expectedError := errors.New("invalid argument")
				stderr := "rmmod: ERROR: Module name is empty"

				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", stderr, expectedError)

				err := h.RmMod(ctx, moduleName)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unload kernel module "))
			})

			It("should handle module name with spaces", func() {
				moduleName := "module with spaces"
				expectedError := errors.New("invalid argument")
				stderr := "rmmod: ERROR: Module name contains invalid characters"

				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", stderr, expectedError)

				err := h.RmMod(ctx, moduleName)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unload kernel module module with spaces"))
			})

			It("should handle very long module names", func() {
				moduleName := "very_long_module_name_that_exceeds_normal_limits_and_tests_boundary_conditions"
				expectedError := errors.New("name too long")
				stderr := "rmmod: ERROR: Module name too long"

				cmdMock.EXPECT().RunCommand(ctx, "rmmod", moduleName).Return("", stderr, expectedError)

				err := h.RmMod(ctx, moduleName)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unload kernel module very_long_module_name_that_exceeds_normal_limits_and_tests_boundary_conditions"))
			})
		})
	})
})
