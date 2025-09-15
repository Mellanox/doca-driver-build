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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
	osMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers/mocks"
)

var _ = Describe("Host", func() {
	Context("GetDebugInfo", func() {
		var (
			h       Interface
			cmdMock *cmdMockPkg.Interface
			osMock  *osMockPkg.OSWrapper
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			h = New(cmdMock, osMock)
		})

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
})
