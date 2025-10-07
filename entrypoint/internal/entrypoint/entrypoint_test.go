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

package entrypoint

import (
	"fmt"
	"os"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	mock "github.com/stretchr/testify/mock"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	driverMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/driver/mocks"
	netconfigMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/netconfig/mocks"
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
	hostMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host/mocks"
	readyMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/ready/mocks"
	udevMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/udev/mocks"
	osMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers/mocks"
)

var _ = Describe("Entrypoint", func() {
	Context("Smoke test", func() {
		var (
			e        *entrypoint
			signalCH chan os.Signal

			readinessMock *readyMockPkg.Interface
			udevMock      *udevMockPkg.Interface
			hostMock      *hostMockPkg.Interface
			cmdMock       *cmdMockPkg.Interface
			osMock        *osMockPkg.OSWrapper
			netconfigMock *netconfigMockPkg.Interface
			driverMock    *driverMockPkg.Interface
		)
		BeforeEach(func() {
			readinessMock = readyMockPkg.NewInterface(GinkgoT())
			udevMock = udevMockPkg.NewInterface(GinkgoT())
			hostMock = hostMockPkg.NewInterface(GinkgoT())
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			netconfigMock = netconfigMockPkg.NewInterface(GinkgoT())
			driverMock = driverMockPkg.NewInterface(GinkgoT())
			e = &entrypoint{
				log: logr.Discard(),
				config: config.Config{
					LockFilePath:                  "/tmp/.lock",
					RestoreDriverOnPodTermination: true,
				},
				containerMode: constants.DriverContainerModeSources,
				drivermgr:     driverMock,
				netconfig:     netconfigMock,
				cmd:           cmdMock,
				readiness:     readinessMock,
				udev:          udevMock,
				os:            osMock,
				host:          hostMock,
			}
			signalCH = make(chan os.Signal, 3)
		})

		It("Succeed", func() {
			osMock.On("MkdirAll", "/tmp", mock.Anything).Return(nil).Once()
			hostMock.On("LsMod", mock.Anything).Return(nil, nil).Once()
			udevMock.On("RemoveRules", mock.Anything).Return(nil).Times(2)

			readinessMock.On("Clear", mock.Anything).Return(nil).Times(2)
			readinessMock.On("Set", mock.Anything).Return(nil).Run(
				func(args mock.Arguments) { signalCH <- syscall.SIGTERM }).Once()

			netconfigMock.On("Save", mock.Anything).Return(nil).Times(2)
			netconfigMock.On("Restore", mock.Anything).Return(nil).Times(2)

			driverMock.On("PreStart", mock.Anything).Return(nil).Once()
			driverMock.On("Build", mock.Anything).Return(nil).Once()
			driverMock.On("Load", mock.Anything).Return(true, nil).Once()
			driverMock.On("Unload", mock.Anything).Return(true, nil).Once()
			driverMock.On("Clear", mock.Anything).Return(nil).Once()

			Expect(e.run(signalCH)).NotTo(HaveOccurred())
		})

		It("preStart failed", func() {
			osMock.On("MkdirAll", "/tmp", mock.Anything).Return(nil).Once()
			udevMock.On("RemoveRules", mock.Anything).Return(nil).Once()
			readinessMock.On("Clear", mock.Anything).Return(nil).Times(1)

			driverMock.On("PreStart", mock.Anything).Return(fmt.Errorf("test")).Once()
			Expect(e.run(signalCH)).To(HaveOccurred())
		})

		It("start failed", func() {
			osMock.On("MkdirAll", "/tmp", mock.Anything).Return(nil).Once()
			hostMock.On("LsMod", mock.Anything).Return(nil, nil).Once()
			udevMock.On("RemoveRules", mock.Anything).Return(nil).Times(2)

			readinessMock.On("Clear", mock.Anything).Return(nil).Times(2)

			netconfigMock.On("Save", mock.Anything).Return(nil).Times(2)
			netconfigMock.On("Restore", mock.Anything).Return(nil).Times(1)

			driverMock.On("PreStart", mock.Anything).Return(nil).Once()
			driverMock.On("Build", mock.Anything).Return(nil).Once()
			driverMock.On("Load", mock.Anything).Return(false, fmt.Errorf("test")).Once()
			driverMock.On("Unload", mock.Anything).Return(true, nil).Once()
			driverMock.On("Clear", mock.Anything).Return(nil).Once()

			Expect(e.run(signalCH)).To(HaveOccurred())
		})

		It("stop failed", func() {
			osMock.On("MkdirAll", "/tmp", mock.Anything).Return(nil).Once()
			hostMock.On("LsMod", mock.Anything).Return(nil, nil).Once()
			udevMock.On("RemoveRules", mock.Anything).Return(nil).Times(2)

			readinessMock.On("Clear", mock.Anything).Return(nil).Times(2)
			readinessMock.On("Set", mock.Anything).Return(nil).Run(
				func(args mock.Arguments) { signalCH <- syscall.SIGTERM }).Once()

			netconfigMock.On("Save", mock.Anything).Return(nil).Times(2)
			netconfigMock.On("Restore", mock.Anything).Return(nil).Times(1)

			driverMock.On("PreStart", mock.Anything).Return(nil).Once()
			driverMock.On("Build", mock.Anything).Return(nil).Once()
			driverMock.On("Load", mock.Anything).Return(true, nil).Once()
			driverMock.On("Unload", mock.Anything).Return(false, fmt.Errorf("test")).Once()

			Expect(e.run(signalCH)).To(HaveOccurred())
		})
	})
})
