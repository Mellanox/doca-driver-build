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

package entrypoint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/gofrs/flock"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/config"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/driver"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/netconfig"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/host"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/ready"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/udev"
	"github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers"
)

// Start the entrypoint manager with file-based locking to ensure only one instance runs at a time.
// Handlers in the entrypoint manager:
//   - preStart: Cleans up, validates, and prepares. If it fails,
//     the process exits immediately without running "stop".
//   - start: Builds and loads the driver after preStart succeeds. If successful,
//     the manager waits for a termination signal. If it fails, "stop" still runs.
//   - stop: Handles unloading the driver and container teardown.
func Run(signalCh chan os.Signal, log logr.Logger, containerMode string, cfg config.Config) error {
	osWrapper := wrappers.NewOS()
	cmdHelper := cmd.New()
	hostHelper := host.New(cmdHelper, osWrapper)
	m := &entrypoint{
		log:           log,
		config:        cfg,
		containerMode: containerMode,
		readiness:     ready.New(cfg.DriverReadyPath, osWrapper),
		udev:          udev.New(cfg.MlxUdevRulesFile, osWrapper),
		host:          hostHelper,
		cmd:           cmdHelper,
		os:            osWrapper,
		netconfig:     netconfig.New(),
		drivermgr:     driver.New(containerMode, cfg, cmdHelper, hostHelper, osWrapper),
	}
	return m.run(signalCh)
}

// entrypoint orchestrates the high-level logic for loading and unloading the driver.
type entrypoint struct {
	log logr.Logger

	config        config.Config
	containerMode string

	drivermgr driver.Interface
	netconfig netconfig.Interface
	cmd       cmd.Interface
	readiness ready.Interface
	udev      udev.Interface
	os        wrappers.OSWrapper
	host      host.Interface
}

// run is an actual implementation of the entrypoint.Run()
func (e *entrypoint) run(signalCh chan os.Signal) error {
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()

	startCtx, startCancel := context.WithCancel(context.Background())
	stopCtx, stopCancel := context.WithCancel(context.Background())
	startCtx = logr.NewContext(startCtx, e.log)
	stopCtx = logr.NewContext(stopCtx, e.log)
	setupSignalHandler(signalCh, []ctxData{{Ctx: startCtx, Cancel: startCancel}, {Ctx: stopCtx, Cancel: stopCancel}})

	e.log.Info("NVIDIA driver container exec preStart")
	if err := e.preStart(startCtx); err != nil {
		e.log.Error(err, "exec preStart failed")
		return err
	}
	e.log.Info("NVIDIA driver container exec start")
	startErr := e.start(startCtx)
	if startErr != nil {
		e.log.Error(err, "exec start failed")
		// explicitly cancel the start context to make sure that the stop context
		// will receive the first sigterm signal
		startCancel()
	} else {
		e.log.Info("configuration done, sleep")
		<-startCtx.Done()
	}
	e.log.Info("NVIDIA driver container exec stop")
	stopErr := e.stop(stopCtx)
	if stopErr != nil {
		e.log.Error(err, "exec stop failed")
	}
	if startErr != nil || stopErr != nil {
		err := fmt.Errorf("startErr: %v, stopErr %v", startErr, stopErr)
		e.log.Error(err, "exec failed")
		return err
	}
	e.log.Info("NVIDIA driver container finished")
	return nil
}

// lock function utilizes a file-based lock to ensure that two entrypoint binaries do not run simultaneously.
// It returns either an unlock function or an error.
func (e *entrypoint) lock() (func(), error) {
	log := e.log.WithValues("lockFile", e.config.LockFilePath)
	if err := e.os.MkdirAll(filepath.Dir(e.config.LockFilePath), 0o755); err != nil {
		log.Error(err, "failed to create base dir for lockfile")
		return nil, err
	}
	fileLock := flock.New(e.config.LockFilePath)
	hasLock, err := fileLock.TryLock()
	if err != nil {
		log.Error(err, "failed to acquired file-based lock")
		return nil, err
	}
	if !hasLock {
		err := fmt.Errorf("NVIDIA driver container is already running")
		log.Error(err, "the container already running")
		return nil, err
	}
	log.V(1).Info("accrued file-based lock")
	return func() {
		log.V(1).Info("release file-based lock")
		if err := fileLock.Unlock(); err != nil {
			log.Error(err, "failed to release file-based lock")
		}
	}, nil
}

// preStart contains logic executed at the beginning of container start,
// failures in this function will not activate the stop handler.
func (e *entrypoint) preStart(ctx context.Context) error {
	if e.log.V(1).Enabled() {
		info, err := e.host.GetDebugInfo(ctx)
		if err != nil {
			e.log.Error(err, "failed to get debug info")
		} else {
			e.log.V(1).Info("debug info: \n" + info)
		}
	}
	if err := e.commonCleanup(ctx); err != nil {
		return err
	}

	if err := e.drivermgr.Prepare(ctx); err != nil {
		return err
	}

	if err := e.handleKernelModules(ctx); err != nil {
		return err
	}

	if err := e.createUDEVRulesIfRequired(ctx); err != nil {
		return err
	}
	if e.containerMode == constants.DriverContainerModeSources {
		if err := e.drivermgr.Build(ctx); err != nil {
			return err
		}
	}
	if err := e.netconfig.Save(ctx); err != nil {
		return err
	}
	return ctx.Err()
}

// start loads the driver and blocks until the context is canceled. The stop handler runs unconditionally after this.
func (e *entrypoint) start(ctx context.Context) error {
	reloaded, err := e.drivermgr.Load(ctx)
	if err != nil {
		return err
	}
	if reloaded {
		// we need to restore configuration only if the driver was loaded
		if err := e.netconfig.Restore(ctx); err != nil {
			return err
		}
	}
	if err := e.readiness.Set(ctx); err != nil {
		return err
	}
	return nil
}

// stop is the termination handler and contains the logic to be executed on container teardown.
func (e *entrypoint) stop(ctx context.Context) error {
	if err := e.commonCleanup(ctx); err != nil {
		return err
	}
	if e.config.RestoreDriverOnPodTermination {
		e.log.Info("restore inbox driver")
		if err := e.netconfig.Save(ctx); err != nil {
			return err
		}
		reloaded, err := e.drivermgr.Unload(ctx)
		if err != nil {
			return err
		}
		if reloaded {
			if err := e.netconfig.Restore(ctx); err != nil {
				return err
			}
		}
	} else {
		e.log.Info("RESTORE_DRIVER_ON_POD_TERMINATION is false, keep existing driver loaded")
	}
	if err := e.drivermgr.Clear(ctx); err != nil {
		return err
	}
	return nil
}

// commonCleanup contains cleanup logic that should be executed on each start and teardown
func (e *entrypoint) commonCleanup(ctx context.Context) error {
	if err := e.readiness.Clear(ctx); err != nil {
		return err
	}
	return e.udev.RemoveRules(ctx)
}

// createUDEVRulesIfRequired generates udev rules to preserve the previous naming scheme for NVIDIA devices,
// if it detects that the inbox driver utilizes the old naming scheme.
func (e *entrypoint) createUDEVRulesIfRequired(ctx context.Context) error {
	if !e.config.CreateIfnamesUdev {
		return nil
	}
	inboxUsesNewNamingScheme, err := e.udev.DevicesUseNewNamingScheme(ctx)
	if err != nil {
		return err
	}
	if !inboxUsesNewNamingScheme {
		e.log.Info("inbox driver uses old naming scheme for interface, create UDEV rules to preserve interface names")
		if err := e.udev.CreateRules(ctx); err != nil {
			return err
		}
	}
	return nil
}

// handleKernelModules function ensures the nvidia_peermem module is unloaded
// and confirms storage modules will unload during openibd restart.
func (e *entrypoint) handleKernelModules(ctx context.Context) error {
	e.log.Info("Verifying loaded modules will not prevent future driver restart")
	loadedModules, err := e.host.LsMod(ctx)
	if err != nil {
		e.log.Error(err, "failed to list loaded kernel modules")
		return err
	}
	nvPeerMemInfo, found := loadedModules["nvidia_peermem"]
	if found {
		if nvPeerMemInfo.RefCount > 0 {
			err := fmt.Errorf("module is used by other modules: %s", nvPeerMemInfo.UsedBy)
			e.log.Error(err, "failed to unload nvidia_peermem module")
			return err
		}
		if err := e.host.RmMod(ctx, "nvidia_peermem"); err != nil {
			e.log.Error(err, "failed to unload nvidia_peermem module")
			return err
		}
		e.log.V(1).Info("nvidia_peermem module unloaded")
	} else {
		e.log.V(1).Info("nvidia_peermem module in not loaded")
	}
	if e.config.UnloadStorageModules {
		// storage modules will be unloaded by the openibd restart, no need to check if they are loaded
		return nil
	}
	for _, mod := range e.config.StorageModules {
		if _, found := loadedModules[mod]; found {
			err = fmt.Errorf("storage modules are loaded for current driver," +
				"terminating prior driver reload failure due to UNLOAD_STORAGE_MODULES not set to \"true\"")
			e.log.Error(err, "kernel modules check failed")
			return err
		}
	}
	return nil
}

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
