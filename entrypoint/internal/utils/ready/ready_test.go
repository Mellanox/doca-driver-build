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

package ready

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	osMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers/mocks"
)

var _ = Describe("Ready", func() {
	Context("Set", func() {
		var (
			r        Interface
			osMock   *osMockPkg.OSWrapper
			testDir  string
			testFile string
		)

		BeforeEach(func() {
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			testDir = "/tmp/test-ready"
			testFile = filepath.Join(testDir, ".driver-ready")
			r = New(testFile, osMock)
		})

		It("should create readiness indicator file successfully", func() {
			// Mock file creation
			mockFile := &os.File{}
			osMock.EXPECT().MkdirAll(testDir, os.FileMode(0755)).Return(nil)
			osMock.EXPECT().Create(testFile).Return(mockFile, nil)

			err := r.Set(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle directory creation failure", func() {
			osMock.EXPECT().MkdirAll(testDir, os.FileMode(0755)).Return(assert.AnError)

			err := r.Set(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(assert.AnError))
		})

		It("should handle file creation failure", func() {
			osMock.EXPECT().MkdirAll(testDir, os.FileMode(0755)).Return(nil)
			osMock.EXPECT().Create(testFile).Return(nil, assert.AnError)

			err := r.Set(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(assert.AnError))
		})

		It("should handle nested directory creation", func() {
			nestedFile := "/tmp/nested/deep/path/.driver-ready"
			nestedDir := "/tmp/nested/deep/path"
			r = New(nestedFile, osMock)

			mockFile := &os.File{}
			osMock.EXPECT().MkdirAll(nestedDir, os.FileMode(0755)).Return(nil)
			osMock.EXPECT().Create(nestedFile).Return(mockFile, nil)

			err := r.Set(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle file in current directory", func() {
			currentDirFile := ".driver-ready"
			r = New(currentDirFile, osMock)

			mockFile := &os.File{}
			osMock.EXPECT().MkdirAll(".", os.FileMode(0755)).Return(nil)
			osMock.EXPECT().Create(currentDirFile).Return(mockFile, nil)

			err := r.Set(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Clear", func() {
		var (
			r        Interface
			osMock   *osMockPkg.OSWrapper
			testFile string
		)

		BeforeEach(func() {
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			testFile = "/tmp/test-ready/.driver-ready"
			r = New(testFile, osMock)
		})

		It("should remove readiness indicator file successfully", func() {
			osMock.EXPECT().RemoveAll(testFile).Return(nil)

			err := r.Clear(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle file removal failure", func() {
			osMock.EXPECT().RemoveAll(testFile).Return(assert.AnError)

			err := r.Clear(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(assert.AnError))
		})

		It("should handle non-existent file gracefully", func() {
			// RemoveAll should not return an error if file doesn't exist
			osMock.EXPECT().RemoveAll(testFile).Return(nil)

			err := r.Clear(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle nested file path", func() {
			nestedFile := "/tmp/nested/deep/path/.driver-ready"
			r = New(nestedFile, osMock)

			osMock.EXPECT().RemoveAll(nestedFile).Return(nil)

			err := r.Clear(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle file in current directory", func() {
			currentDirFile := ".driver-ready"
			r = New(currentDirFile, osMock)

			osMock.EXPECT().RemoveAll(currentDirFile).Return(nil)

			err := r.Clear(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Integration", func() {
		var (
			r        Interface
			osMock   *osMockPkg.OSWrapper
			testFile string
		)

		BeforeEach(func() {
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			testFile = "/tmp/test-ready/.driver-ready"
			r = New(testFile, osMock)
		})

		It("should handle set and clear operations in sequence", func() {
			// Set operation
			mockFile := &os.File{}
			osMock.EXPECT().MkdirAll("/tmp/test-ready", os.FileMode(0755)).Return(nil)
			osMock.EXPECT().Create(testFile).Return(mockFile, nil)

			err := r.Set(context.Background())
			Expect(err).ToNot(HaveOccurred())

			// Clear operation
			osMock.EXPECT().RemoveAll(testFile).Return(nil)

			err = r.Clear(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle multiple set operations", func() {
			mockFile := &os.File{}
			osMock.EXPECT().MkdirAll("/tmp/test-ready", os.FileMode(0755)).Return(nil).Times(2)
			osMock.EXPECT().Create(testFile).Return(mockFile, nil).Times(2)

			err := r.Set(context.Background())
			Expect(err).ToNot(HaveOccurred())

			err = r.Set(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle multiple clear operations", func() {
			osMock.EXPECT().RemoveAll(testFile).Return(nil).Times(2)

			err := r.Clear(context.Background())
			Expect(err).ToNot(HaveOccurred())

			err = r.Clear(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
