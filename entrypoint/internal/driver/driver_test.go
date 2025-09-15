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

package driver

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
	hostMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host/mocks"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

var _ = Describe("Driver OFED Blacklist", func() {
	Context("generateOfedModulesBlacklist", func() {
		var (
			dm       *driverMgr
			cmdMock  *cmdMockPkg.Interface
			hostMock *hostMockPkg.Interface
			ctx      context.Context
			tempDir  string
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			ctx = context.Background()
			tempDir = GinkgoT().TempDir()
		})

		It("should create blacklist file with all modules", func() {
			blacklistFile := filepath.Join(tempDir, "blacklist-ofed-modules.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules: []string{
					"mlx5_core",
					"mlx5_ib",
					"ib_umad",
					"ib_uverbs",
					"ib_ipoib",
					"rdma_cm",
					"rdma_ucm",
					"ib_core",
					"ib_cm",
				},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_ib"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_umad"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_uverbs"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_ipoib"))
			Expect(contentStr).To(ContainSubstring("blacklist rdma_cm"))
			Expect(contentStr).To(ContainSubstring("blacklist rdma_ucm"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_core"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_cm"))
		})

		It("should handle empty modules list", func() {
			blacklistFile := filepath.Join(tempDir, "empty-blacklist.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content - should only have header
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))

			// Count blacklist lines - should be 0 (only header comment)
			lines := strings.Split(contentStr, "\n")
			blacklistLines := 0
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "blacklist") {
					blacklistLines++
				}
			}
			Expect(blacklistLines).To(Equal(0))
		})

		It("should skip empty or whitespace-only module names", func() {
			blacklistFile := filepath.Join(tempDir, "filtered-blacklist.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules: []string{
					"mlx5_core",
					"",    // empty string
					"   ", // whitespace only
					"mlx5_ib",
					"\t\n", // whitespace with newlines
					"ib_umad",
				},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_ib"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_umad"))

			// Count blacklist lines - should be 3 (empty and whitespace entries should be skipped)
			lines := strings.Split(contentStr, "\n")
			blacklistLines := 0
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "blacklist") {
					blacklistLines++
				}
			}
			Expect(blacklistLines).To(Equal(3))
		})

		It("should handle single module", func() {
			blacklistFile := filepath.Join(tempDir, "single-module.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))

			// Count blacklist lines - should be 1
			lines := strings.Split(contentStr, "\n")
			blacklistLines := 0
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "blacklist") {
					blacklistLines++
				}
			}
			Expect(blacklistLines).To(Equal(1))
		})
	})

	Context("removeOfedModulesBlacklist", func() {
		var (
			dm       *driverMgr
			cmdMock  *cmdMockPkg.Interface
			hostMock *hostMockPkg.Interface
			ctx      context.Context
			tempDir  string
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			ctx = context.Background()
			tempDir = GinkgoT().TempDir()
		})

		It("should remove existing blacklist file", func() {
			blacklistFile := filepath.Join(tempDir, "blacklist.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// First create the file
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Now remove it
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file is removed
			_, err = os.Stat(blacklistFile)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should handle file not existing gracefully", func() {
			blacklistFile := filepath.Join(tempDir, "nonexistent.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Try to remove non-existent file
			err := dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle multiple remove operations", func() {
			blacklistFile := filepath.Join(tempDir, "multi-remove.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Create file
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Remove it multiple times - should not error
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Integration tests", func() {
		var (
			dm       *driverMgr
			cmdMock  *cmdMockPkg.Interface
			hostMock *hostMockPkg.Interface
			ctx      context.Context
			tempDir  string
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			ctx = context.Background()
			tempDir = GinkgoT().TempDir()
		})

		It("should create and then remove blacklist file", func() {
			blacklistFile := filepath.Join(tempDir, "integration-test.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules:     []string{"mlx5_core", "mlx5_ib", "ib_umad"},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Test creation
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists and has correct content
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_ib"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_umad"))

			// Test removal
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file is removed
			_, err = os.Stat(blacklistFile)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should handle complex module list with mixed valid and invalid entries", func() {
			blacklistFile := filepath.Join(tempDir, "complex-test.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules: []string{
					"mlx5_core",
					"", // empty string
					"mlx5_ib",
					"   ", // whitespace only
					"ib_umad",
					"\t\n", // whitespace with newlines
					"ib_uverbs",
					"", // another empty string
					"rdma_cm",
				},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Test creation
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_core"))
			Expect(contentStr).To(ContainSubstring("blacklist mlx5_ib"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_umad"))
			Expect(contentStr).To(ContainSubstring("blacklist ib_uverbs"))
			Expect(contentStr).To(ContainSubstring("blacklist rdma_cm"))

			// Count blacklist lines - should be 5 (empty and whitespace entries should be skipped)
			lines := strings.Split(contentStr, "\n")
			blacklistLines := 0
			for _, line := range lines {
				if strings.HasPrefix(strings.TrimSpace(line), "blacklist") {
					blacklistLines++
				}
			}
			Expect(blacklistLines).To(Equal(5))

			// Test removal
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file is removed
			_, err = os.Stat(blacklistFile)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("should handle default configuration values", func() {
			blacklistFile := filepath.Join(tempDir, "default-config.conf")
			cfg := config.Config{
				OfedBlacklistModulesFile: blacklistFile,
				OfedBlacklistModules: []string{
					"mlx5_core",
					"mlx5_ib",
					"ib_umad",
					"ib_uverbs",
					"ib_ipoib",
					"rdma_cm",
					"rdma_ucm",
					"ib_core",
					"ib_cm",
				},
			}

			dm = &driverMgr{
				cfg:  cfg,
				cmd:  cmdMock,
				host: hostMock,
				os:   wrappers.NewOS(),
			}

			// Test creation with default modules
			err := dm.generateOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file exists
			_, err = os.Stat(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			// Read and verify content matches expected default modules
			content, err := os.ReadFile(blacklistFile)
			Expect(err).ToNot(HaveOccurred())

			contentStr := string(content)
			Expect(contentStr).To(ContainSubstring("# blacklist ofed-related modules on host to prevent inbox or host OFED driver loading"))

			// Verify all expected modules are present
			expectedModules := []string{
				"mlx5_core", "mlx5_ib", "ib_umad", "ib_uverbs", "ib_ipoib",
				"rdma_cm", "rdma_ucm", "ib_core", "ib_cm",
			}
			for _, module := range expectedModules {
				Expect(contentStr).To(ContainSubstring("blacklist " + module))
			}

			// Test removal
			err = dm.removeOfedModulesBlacklist(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Verify file is removed
			_, err = os.Stat(blacklistFile)
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
	})
})
