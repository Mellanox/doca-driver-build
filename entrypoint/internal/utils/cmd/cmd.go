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

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
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
	// NotFound checks if the error is "command not found" error.
	NotFound(err error) bool
}

type cmd struct{}

// formatCommandOutput formats command output for logging, making carriage returns visible
func formatCommandOutput(output string) string {
	// Replace carriage returns with [CR] for visibility
	formatted := strings.ReplaceAll(output, "\r", "[CR]")
	// Note: We don't replace \n here because the logger will handle newlines naturally
	// when the output is displayed in the log message
	return formatted
}

// RunCommand is the default implementation of the cmd.Interface.
func (c *cmd) RunCommand(ctx context.Context, command string, args ...string) (string, string, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("RunCommand()", "command", command, "args", args)
	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, command, args...)
	// Ensure child process is killed when context is canceled
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Format output for logging
	stdoutFormatted := formatCommandOutput(stdout.String())
	stderrFormatted := formatCommandOutput(stderr.String())

	// Log with actual line breaks by using string formatting instead of structured logging
	logMessage := fmt.Sprintf("RunCommand() command=%s args=%v error=%v", command, args, err)
	if stdoutFormatted != "" {
		logMessage += fmt.Sprintf("\nstdout:\n%s", stdoutFormatted)
	}
	if stderrFormatted != "" {
		logMessage += fmt.Sprintf("\nstderr:\n%s", stderrFormatted)
	}

	log.V(1).Info(logMessage)
	return stdout.String(), stderr.String(), err
}

// NotFound is the default implementation of the cmd.Interface.
func (c *cmd) NotFound(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.ExitStatus() == 127 {
			return true
		}
	}
	return false
}
