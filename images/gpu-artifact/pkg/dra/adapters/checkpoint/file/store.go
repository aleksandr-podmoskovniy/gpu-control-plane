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

package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
)

// Store persists checkpoints in a single file.
type Store struct {
	path string
}

// New constructs a new file-based checkpoint store.
func New(path string) *Store {
	return &Store{path: path}
}

// Load reads a checkpoint from disk.
func (s *Store) Load(_ context.Context) (domain.PrepareCheckpoint, error) {
	if s == nil || s.path == "" {
		return domain.PrepareCheckpoint{}, errors.New("checkpoint path is not configured")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return domain.PrepareCheckpoint{}, nil
		}
		return domain.PrepareCheckpoint{}, fmt.Errorf("read checkpoint: %w", err)
	}
	if len(data) == 0 {
		return domain.PrepareCheckpoint{}, nil
	}
	var checkpoint domain.PrepareCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return domain.PrepareCheckpoint{}, fmt.Errorf("decode checkpoint: %w", err)
	}
	return checkpoint, nil
}

// Save writes a checkpoint to disk atomically.
func (s *Store) Save(_ context.Context, checkpoint domain.PrepareCheckpoint) error {
	if s == nil || s.path == "" {
		return errors.New("checkpoint path is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("ensure checkpoint dir: %w", err)
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	return writeAtomically(s.path, data, 0o644)
}

func writeAtomically(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename checkpoint: %w", err)
	}
	return nil
}
