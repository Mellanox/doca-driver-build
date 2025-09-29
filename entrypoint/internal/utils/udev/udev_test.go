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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	osMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers/mocks"
)

// mockFileInfo is a simple mock implementation of os.FileInfo
type mockFileInfo struct{}

func (m mockFileInfo) Name() string       { return "mock" }
func (m mockFileInfo) Size() int64        { return 0 }
func (m mockFileInfo) Mode() os.FileMode  { return 0 }
func (m mockFileInfo) ModTime() time.Time { return time.Now() }
func (m mockFileInfo) IsDir() bool        { return false }
func (m mockFileInfo) Sys() interface{}   { return nil }

var _ = Describe("Udev", func() {
	Context("CreateRules", func() {
		var (
			u        Interface
			osMock   *osMockPkg.OSWrapper
			testPath string
		)

		BeforeEach(func() {
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			testPath = "/host/etc/udev/rules.d/77-mlnx-net-names.rules"
			u = New(testPath, osMock)
		})

		It("should create udev rules file successfully", func() {
			expectedContent := `ACTION!="add", GOTO="mlnx_ofed_name_end"
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

			osMock.EXPECT().WriteFile(testPath, []byte(expectedContent), os.FileMode(0o644)).Return(nil)

			err := u.CreateRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle file creation failure", func() {
			osMock.EXPECT().WriteFile(testPath, mock.AnythingOfType("[]uint8"), os.FileMode(0o644)).Return(assert.AnError)

			err := u.CreateRules(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(assert.AnError))
		})

		It("should handle different file paths", func() {
			customPath := "/custom/path/to/udev/rules.d/99-custom.rules"
			u = New(customPath, osMock)

			osMock.EXPECT().WriteFile(customPath, mock.AnythingOfType("[]uint8"), os.FileMode(0o644)).Return(nil)

			err := u.CreateRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle nested directory paths", func() {
			nestedPath := "/host/etc/udev/rules.d/nested/deep/path/77-mlnx-net-names.rules"
			u = New(nestedPath, osMock)

			osMock.EXPECT().WriteFile(nestedPath, mock.AnythingOfType("[]uint8"), os.FileMode(0o644)).Return(nil)

			err := u.CreateRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create rules with correct content structure", func() {
			var capturedContent []byte
			osMock.EXPECT().WriteFile(testPath, mock.AnythingOfType("[]uint8"), os.FileMode(0o644)).Run(func(name string, data []byte, perm os.FileMode) {
				capturedContent = data
			}).Return(nil)

			err := u.CreateRules(context.Background())
			Expect(err).ToNot(HaveOccurred())

			content := string(capturedContent)
			Expect(content).To(ContainSubstring(`ACTION!="add", GOTO="mlnx_ofed_name_end"`))
			Expect(content).To(ContainSubstring(`SUBSYSTEM!="net", GOTO="mlnx_ofed_name_end"`))
			Expect(content).To(ContainSubstring(`DRIVERS=="mlx5_core", ENV{ID_NET_NAME_PATH}!=""`))
			Expect(content).To(ContainSubstring(`DRIVERS=="mlx5_core", ENV{ID_NET_NAME_SLOT}!=""`))
			Expect(content).To(ContainSubstring(`LABEL="mlnx_ofed_name_end"`))
			Expect(content).To(ContainSubstring(`sed -r -e s/np[01]$// -e s/np[01]v/v/`))
		})

		It("should handle multiple calls to CreateRules", func() {
			osMock.EXPECT().WriteFile(testPath, mock.AnythingOfType("[]uint8"), os.FileMode(0o644)).Return(nil).Times(2)

			err := u.CreateRules(context.Background())
			Expect(err).ToNot(HaveOccurred())

			err = u.CreateRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

	})

	Context("RemoveRules", func() {
		var (
			u        Interface
			osMock   *osMockPkg.OSWrapper
			testPath string
		)

		BeforeEach(func() {
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			testPath = "/host/etc/udev/rules.d/77-mlnx-net-names.rules"
			u = New(testPath, osMock)
		})

		It("should remove udev rules file when it exists", func() {
			// Mock file exists check
			osMock.EXPECT().Stat(testPath).Return(mockFileInfo{}, nil)
			osMock.EXPECT().RemoveAll(testPath).Return(nil)

			err := u.RemoveRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle file not existing gracefully", func() {
			// Mock file doesn't exist (Stat returns os.ErrNotExist)
			osMock.EXPECT().Stat(testPath).Return(nil, os.ErrNotExist)

			err := u.RemoveRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle file removal failure", func() {
			// Mock file exists but removal fails
			osMock.EXPECT().Stat(testPath).Return(mockFileInfo{}, nil)
			osMock.EXPECT().RemoveAll(testPath).Return(assert.AnError)

			err := u.RemoveRules(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(assert.AnError))
		})

		It("should handle different file paths", func() {
			customPath := "/custom/path/to/udev/rules.d/99-custom.rules"
			u = New(customPath, osMock)

			osMock.EXPECT().Stat(customPath).Return(mockFileInfo{}, nil)
			osMock.EXPECT().RemoveAll(customPath).Return(nil)

			err := u.RemoveRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle nested directory paths", func() {
			nestedPath := "/host/etc/udev/rules.d/nested/deep/path/77-mlnx-net-names.rules"
			u = New(nestedPath, osMock)

			osMock.EXPECT().Stat(nestedPath).Return(mockFileInfo{}, nil)
			osMock.EXPECT().RemoveAll(nestedPath).Return(nil)

			err := u.RemoveRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle multiple calls to RemoveRules", func() {
			// First call - file exists
			osMock.EXPECT().Stat(testPath).Return(mockFileInfo{}, nil)
			osMock.EXPECT().RemoveAll(testPath).Return(nil)

			err := u.RemoveRules(context.Background())
			Expect(err).ToNot(HaveOccurred())

			// Second call - file no longer exists
			osMock.EXPECT().Stat(testPath).Return(nil, os.ErrNotExist)

			err = u.RemoveRules(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle Stat error other than file not found", func() {
			// Mock Stat returning a different error (e.g., permission denied)
			osMock.EXPECT().Stat(testPath).Return(nil, assert.AnError)

			err := u.RemoveRules(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(assert.AnError))
		})
	})
})
