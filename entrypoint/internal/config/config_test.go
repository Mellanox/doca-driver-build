// Copyright 2025 NVIDIA CORPORATION & AFFILIATES
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	// NVIDIA_NIC_DRIVER_VER is required by env parsing, so we must set it.
	BeforeEach(func() {
		os.Setenv("NVIDIA_NIC_DRIVER_VER", "25.04-0.6.0.0")
	})

	AfterEach(func() {
		os.Unsetenv("NVIDIA_NIC_DRIVER_VER")
		os.Unsetenv("OFED_BLACKLIST_MODULES")
	})

	Context("OfedBlacklistModules", func() {
		It("should use DefaultOfedBlacklistModules when env var is not set", func() {
			os.Unsetenv("OFED_BLACKLIST_MODULES")

			cfg, err := GetConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.OfedBlacklistModules).To(Equal(DefaultOfedBlacklistModules))
		})

		It("should append extra modules to defaults when env var has additional modules", func() {
			os.Setenv("OFED_BLACKLIST_MODULES", "my_mod1:my_mod2")

			cfg, err := GetConfig()
			Expect(err).NotTo(HaveOccurred())
			expected := append([]string{}, DefaultOfedBlacklistModules...)
			expected = append(expected, "my_mod1", "my_mod2")
			Expect(cfg.OfedBlacklistModules).To(Equal(expected))
		})

		It("should not duplicate modules when env var overlaps with defaults", func() {
			os.Setenv("OFED_BLACKLIST_MODULES", "mlx5_core:my_custom_mod:ib_core")

			cfg, err := GetConfig()
			Expect(err).NotTo(HaveOccurred())
			// mlx5_core and ib_core are already in defaults, so only my_custom_mod is new
			expected := append([]string{}, DefaultOfedBlacklistModules...)
			expected = append(expected, "my_custom_mod")
			Expect(cfg.OfedBlacklistModules).To(Equal(expected))
		})

		It("should return defaults when env var is set to empty string", func() {
			os.Setenv("OFED_BLACKLIST_MODULES", "")

			cfg, err := GetConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.OfedBlacklistModules).To(Equal(DefaultOfedBlacklistModules))
		})
	})

	Context("mergeModules", func() {
		It("should deduplicate preserving order", func() {
			result := mergeModules(
				[]string{"a", "b", "c"},
				[]string{"b", "d", "a"},
			)
			Expect(result).To(Equal([]string{"a", "b", "c", "d"}))
		})

		It("should handle empty additional list", func() {
			result := mergeModules([]string{"a", "b"}, nil)
			Expect(result).To(Equal([]string{"a", "b"}))
		})

		It("should skip empty strings", func() {
			result := mergeModules([]string{"a", ""}, []string{"", "b"})
			Expect(result).To(Equal([]string{"a", "b"}))
		})
	})
})
