/*
Copyright 2025 Flant JSC

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

package prestart

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

// FS abstracts filesystem operations for testing.
type FS interface {
	Symlink(oldname, newname string) error
	Stat(name string) (os.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
}

// ExecFunc runs a command and returns its exit code.
type ExecFunc func(ctx context.Context, path string, env []string, stdout, stderr io.Writer) (int, error)

type osFS struct{}

func (osFS) Symlink(oldname, newname string) error      { return os.Symlink(oldname, newname) }
func (osFS) Stat(name string) (os.FileInfo, error)      { return os.Stat(name) }
func (osFS) ReadDir(name string) ([]os.DirEntry, error) { return os.ReadDir(name) }

func execCommand(ctx context.Context, path string, env []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.CommandContext(ctx, path)
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), err
	}

	return 127, err
}
