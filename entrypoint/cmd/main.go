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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/dtk"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/entrypoint"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/version"
)

type ctxData struct {
	//nolint:containedctx
	Ctx    context.Context
	Cancel context.CancelFunc
}

// setupSignalHandler takes a signal channel and contexts with cancel functions.
// It starts a goroutine that cancels the first uncanceled context on receiving a signal,
// if no uncanceled context exists, it exits the application with code 1.
func setupSignalHandler(ch chan os.Signal, ctxs []ctxData) {
	go func() {
	OUT:
		for {
			<-ch
			for _, ctx := range ctxs {
				if ctx.Ctx.Err() != nil {
					// context is already canceled, try next one
					continue
				}
				ctx.Cancel()
				continue OUT
			}
			os.Exit(1)
		}
	}()
}

func main() {
	cfg, err := config.GetConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to parse configuration: %v\n", err)
		os.Exit(1)
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
	if containerMode == constants.DriverContainerModeDtkBuild {
		// Use a context that is canceled on signal
		ctx, cancel := context.WithCancel(context.Background())
		// Attach logger to context
		ctx = logr.NewContext(ctx, log)
		setupSignalHandler(getSignalChannel(), []ctxData{{Ctx: ctx, Cancel: cancel}})

		if err := dtk.RunBuild(ctx, log, cfg, cmd.New()); err != nil {
			log.Error(err, "DTK Build failed")
			os.Exit(1)
		}
		return
	}

	if err := entrypoint.Run(getSignalChannel(), log, containerMode, cfg); err != nil {
		log.Error(err, "Entrypoint Run failed")
		os.Exit(1)
	}
}

func getContainerMode() (string, error) {
	flag.Parse()
	containerMode := flag.Arg(0)
	if flag.NArg() != 1 ||
		(containerMode != constants.DriverContainerModePrecompiled &&
			containerMode != constants.DriverContainerModeSources &&
			containerMode != constants.DriverContainerModeDtkBuild) {
		return "", fmt.Errorf("container mode argument has invalid value %s, supported values: %s, %s, %s",
			containerMode, constants.DriverContainerModePrecompiled, constants.DriverContainerModeSources, constants.DriverContainerModeDtkBuild)
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
			// Create directory if it doesn't exist
			logDir := filepath.Dir(cfg.DebugLogFile)
			if err := os.MkdirAll(logDir, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "WARNING: failed to create log directory %s: %v\n", logDir, err)
			}
			logConfig.OutputPaths = append(logConfig.OutputPaths, cfg.DebugLogFile)
			logConfig.ErrorOutputPaths = append(logConfig.ErrorOutputPaths, cfg.DebugLogFile)
		}
	}
	zapLog, err := logConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: can't init the logger %v\n", err)
		os.Exit(1)
	}
	return zapr.NewLogger(zapLog)
}

func getSignalChannel() chan os.Signal {
	ch := make(chan os.Signal, 3)
	signal.Notify(ch, []os.Signal{os.Interrupt, syscall.SIGTERM}...)
	return ch
}
