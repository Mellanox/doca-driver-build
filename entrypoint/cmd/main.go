/*
 Copyright 2024, NVIDIA CORPORATION & AFFILIATES

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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/entrypoint"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/version"
)

func main() {
	cfg, err := config.GetConfig()
	if err != nil {
		panic("failed to parse configuration" + err.Error())
	}

	log := getLogger(cfg)
	log.Info("entrypoint", "version", version.GetVersionString())

	log.Info(fmt.Sprintf("Container full version: %s-%s", cfg.NvidiaNicDriverVer, cfg.NvidiaNicContainerVer))

	if log.V(1).Enabled() {
		//nolint:errchkjson
		data, _ := json.MarshalIndent(cfg, "", "  ")
		log.V(1).Info("driver container config: \n" + string(data))
	}
	containerMode, err := getContainerMode()
	if err != nil {
		log.Error(err, "can't determine container execution mode")
		os.Exit(1)
	}
	log.Info("start manager", "mode", containerMode)
	if err := entrypoint.Run(getSignalChannel(), log, containerMode, cfg); err != nil {
		os.Exit(1)
	}
}

func getContainerMode() (string, error) {
	flag.Parse()
	containerMode := flag.Arg(0)
	if flag.NArg() != 1 ||
		(containerMode != constants.DriverContainerModePrecompiled && containerMode != string(constants.DriverContainerModeSources)) {
		return "", fmt.Errorf("container mode argument has invalid value %s, supported values: %s, %s",
			containerMode, constants.DriverContainerModePrecompiled, constants.DriverContainerModeSources)
	}
	return containerMode, nil
}

func getLogger(cfg config.Config) logr.Logger {
	logConfig := zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Encoding:          "console",
		DisableStacktrace: true,
		EncoderConfig:     zap.NewDevelopmentEncoderConfig(),
		OutputPaths:       []string{"stderr"},
		ErrorOutputPaths:  []string{"stderr"},
	}

	if cfg.EntrypointDebug {
		logConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
		if cfg.DebugLogFile != "" {
			logConfig.OutputPaths = append(logConfig.OutputPaths, cfg.DebugLogFile)
			logConfig.ErrorOutputPaths = append(logConfig.ErrorOutputPaths, cfg.DebugLogFile)
		}
	}
	zapLog, err := logConfig.Build()
	if err != nil {
		panic("can't init the logger: " + err.Error())
	}
	return zapr.NewLogger(zapLog)
}

func getSignalChannel() chan os.Signal {
	ch := make(chan os.Signal, 3)
	signal.Notify(ch, []os.Signal{os.Interrupt, syscall.SIGTERM}...)
	return ch
}
