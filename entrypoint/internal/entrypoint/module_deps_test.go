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

package entrypoint

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ModuleDeps", func() {
	var tempDir string

	BeforeEach(func() {
		tempDir = GinkgoT().TempDir()
	})

	Context("ParseProcModules", func() {
		It("should parse valid /proc/modules lines", func() {
			content := `mlx5_core 1234567 2 mlx5_ib,mlx5_vdpa, Live 0xffffffffc0000000
mlx5_ib 234567 1 rdma_cm, Live 0xffffffffc0100000
ib_core 345678 3 mlx5_ib,rdma_cm,ib_umad, Live 0xffffffffc0200000
nvidia_peermem 12345 0 - Live 0xffffffffc0300000
`
			procFile := filepath.Join(tempDir, "modules")
			Expect(os.WriteFile(procFile, []byte(content), 0o644)).To(Succeed())

			modules, err := ParseProcModules(procFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(modules).To(HaveLen(4))

			Expect(modules["mlx5_core"].UserCount).To(Equal(2))
			Expect(modules["mlx5_core"].DependsOn).To(ConsistOf("mlx5_ib", "mlx5_vdpa"))

			Expect(modules["mlx5_ib"].UserCount).To(Equal(1))
			Expect(modules["mlx5_ib"].DependsOn).To(ConsistOf("rdma_cm"))

			Expect(modules["ib_core"].UserCount).To(Equal(3))
			Expect(modules["ib_core"].DependsOn).To(ConsistOf("mlx5_ib", "rdma_cm", "ib_umad"))

			Expect(modules["nvidia_peermem"].UserCount).To(Equal(0))
			Expect(modules["nvidia_peermem"].DependsOn).To(BeEmpty())
		})

		It("should handle malformed lines gracefully", func() {
			content := `valid_mod 1234 1 dep1, Live 0x0
short_line 1234
`
			procFile := filepath.Join(tempDir, "modules")
			Expect(os.WriteFile(procFile, []byte(content), 0o644)).To(Succeed())

			modules, err := ParseProcModules(procFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(modules).To(HaveLen(1))
			Expect(modules).To(HaveKey("valid_mod"))
		})

		It("should handle empty file", func() {
			procFile := filepath.Join(tempDir, "modules")
			Expect(os.WriteFile(procFile, []byte(""), 0o644)).To(Succeed())

			modules, err := ParseProcModules(procFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(modules).To(BeEmpty())
		})

		It("should return error for non-existent file", func() {
			_, err := ParseProcModules(filepath.Join(tempDir, "nonexistent"))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("ValidateUnloadSafety", func() {
		var sysModDir string

		BeforeEach(func() {
			sysModDir = filepath.Join(tempDir, "sys_module")
			Expect(os.MkdirAll(sysModDir, 0o755)).To(Succeed())
		})

		// createHolders creates a /sys/module/<mod>/holders/ directory with the given holder entries.
		createHolders := func(mod string, holders []string) {
			holdersDir := filepath.Join(sysModDir, mod, "holders")
			Expect(os.MkdirAll(holdersDir, 0o755)).To(Succeed())
			for _, h := range holders {
				// Create empty files to mimic sysfs holder entries
				Expect(os.WriteFile(filepath.Join(holdersDir, h), []byte{}, 0o644)).To(Succeed())
			}
		}

		It("should fail when refcount exceeds holder count — reporting userspace users and holder names", func() {
			// refcount=2, 1 holder → 1 unknown userspace user
			createHolders("ib_umad", []string{"mlx5_ib"})
			modules := map[string]ModuleInfo{
				"ib_umad": {Name: "ib_umad", UserCount: 2, DependsOn: []string{"ib_core"}},
			}

			err := ValidateUnloadSafety([]string{"ib_umad"}, modules, sysModDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("1 unknown userspace process"))
			Expect(err.Error()).To(ContainSubstring("mlx5_ib"))
			Expect(err.Error()).To(ContainSubstring("lsof /dev/infiniband/*"))
		})

		It("should pass when refcount matches holder count — all users accounted for", func() {
			// refcount=1, 1 holder → passes (all accounted for)
			createHolders("ib_umad", []string{"mlx5_ib"})
			modules := map[string]ModuleInfo{
				"ib_umad": {Name: "ib_umad", UserCount: 1, DependsOn: []string{"ib_core"}},
			}

			err := ValidateUnloadSafety([]string{"ib_umad"}, modules, sysModDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass when refcount is zero", func() {
			createHolders("nvidia_peermem", nil)
			modules := map[string]ModuleInfo{
				"nvidia_peermem": {Name: "nvidia_peermem", UserCount: 0, DependsOn: nil},
			}

			err := ValidateUnloadSafety([]string{"nvidia_peermem"}, modules, sysModDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip modules that are not loaded", func() {
			modules := map[string]ModuleInfo{}
			err := ValidateUnloadSafety([]string{"not_loaded_mod"}, modules, sysModDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should treat missing holders dir as holderCount=0 and compare against refcount", func() {
			// holders dir doesn't exist → holderCount=0
			// refcount=1 → 1 unknown userspace user
			modules := map[string]ModuleInfo{
				"some_mod": {Name: "some_mod", UserCount: 1, DependsOn: nil},
			}

			err := ValidateUnloadSafety([]string{"some_mod"}, modules, sysModDir)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("1 unknown userspace process"))
		})

		It("should pass when holders dir does not exist and refcount is zero", func() {
			modules := map[string]ModuleInfo{
				"clean_mod": {Name: "clean_mod", UserCount: 0, DependsOn: nil},
			}

			err := ValidateUnloadSafety([]string{"clean_mod"}, modules, sysModDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should pass with empty targets", func() {
			modules := map[string]ModuleInfo{
				"lustre": {Name: "lustre", UserCount: 5, DependsOn: []string{"ib_core"}},
			}
			err := ValidateUnloadSafety([]string{}, modules, sysModDir)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("ResolveUnloadOrder", func() {
		It("should resolve a linear chain", func() {
			// lustre's DependsOn contains ko2iblnd (ko2iblnd uses lustre)
			// So ko2iblnd should be unloaded first (leaf-first)
			modules := map[string]ModuleInfo{
				"lustre": {
					Name:       "lustre",
					UserCount:  1,
					DependsOn: []string{"ko2iblnd"},
				},
				"ko2iblnd": {
					Name:       "ko2iblnd",
					UserCount:  0,
					DependsOn: nil,
				},
			}

			order, err := ResolveUnloadOrder([]string{"lustre", "ko2iblnd"}, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(HaveLen(2))
			// ko2iblnd should come before lustre (leaf-first)
			Expect(order[0]).To(Equal("ko2iblnd"))
			Expect(order[1]).To(Equal("lustre"))
		})

		It("should resolve a diamond dependency", func() {
			// A is used by B and C; B and C are used by D
			modules := map[string]ModuleInfo{
				"A": {Name: "A", UserCount: 2, DependsOn: []string{"B", "C"}},
				"B": {Name: "B", UserCount: 1, DependsOn: []string{"D"}},
				"C": {Name: "C", UserCount: 1, DependsOn: []string{"D"}},
				"D": {Name: "D", UserCount: 0, DependsOn: nil},
			}

			order, err := ResolveUnloadOrder([]string{"A", "B", "C", "D"}, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(HaveLen(4))
			// D must be first (leaf), A must be last
			Expect(order[0]).To(Equal("D"))
			Expect(order[3]).To(Equal("A"))
		})

		It("should handle a single module", func() {
			modules := map[string]ModuleInfo{
				"single": {Name: "single", UserCount: 0, DependsOn: nil},
			}

			order, err := ResolveUnloadOrder([]string{"single"}, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"single"}))
		})

		It("should handle modules not in /proc/modules", func() {
			modules := map[string]ModuleInfo{}
			order, err := ResolveUnloadOrder([]string{"missing"}, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"missing"}))
		})

		It("should handle empty targets", func() {
			modules := map[string]ModuleInfo{
				"something": {Name: "something", UserCount: 0, DependsOn: nil},
			}
			order, err := ResolveUnloadOrder([]string{}, modules)
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(BeEmpty())
		})
	})
})
