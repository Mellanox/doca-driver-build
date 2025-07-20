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

package cmd

import (
	"bytes"
	"context"
	"os/exec"
	"syscall"

	"github.com/go-logr/logr"
)

// New initialize default implementation of the cmd.Interface.
func New() Interface {
	return &cmd{}
}

// Interface is the interface exposed by the cmd package.
type Interface interface {
	// RunCommand runs a command.
	RunCommand(ctx context.Context, command string, args ...string) (string, string, error)
	// IsCommandNotFound checks if the error is "command not found" error.
	IsCommandNotFound(err error) bool
}

type cmd struct{}

// RunCommand is the default implementation of the cmd.Interface.
func (c *cmd) RunCommand(ctx context.Context, command string, args ...string) (string, string, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("RunCommand()", "command", command, "args", args)
	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	log.V(1).Info("RunCommand()", "output", stdout.String(), "error", err)
	return stdout.String(), stderr.String(), err
}

// IsCommandNotFound is the default implementation of the cmd.Interface.
func (c *cmd) IsCommandNotFound(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == 127 {
			return true
		}
	}
	return false
}
