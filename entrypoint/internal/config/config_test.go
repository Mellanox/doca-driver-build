// Copyright 2026 NVIDIA CORPORATION & AFFILIATES
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
		os.Unsetenv("UNLOAD_THIRD_PARTY_RDMA_MODULES")
	})

	Context("UnloadThirdPartyRdmaModules", func() {
		It("should default to false when UNLOAD_THIRD_PARTY_RDMA_MODULES is not set", func() {
			os.Unsetenv("UNLOAD_THIRD_PARTY_RDMA_MODULES")

			cfg, err := GetConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.UnloadThirdPartyRdmaModules).To(BeFalse())
		})

		It("should be true when set to \"true\"", func() {
			os.Setenv("UNLOAD_THIRD_PARTY_RDMA_MODULES", "true")

			cfg, err := GetConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.UnloadThirdPartyRdmaModules).To(BeTrue())
		})

		It("should be false when set to \"false\"", func() {
			os.Setenv("UNLOAD_THIRD_PARTY_RDMA_MODULES", "false")

			cfg, err := GetConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.UnloadThirdPartyRdmaModules).To(BeFalse())
		})
	})
})
