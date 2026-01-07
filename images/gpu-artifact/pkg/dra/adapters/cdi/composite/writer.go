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

package composite

import (
	"context"
	"errors"
	"fmt"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
)

// Writer selects a CDI writer based on the prepare request.
type Writer struct {
	standard ports.CDIWriter
	vfio     ports.CDIWriter
}

// New constructs a CDI writer selector.
func New(standard, vfio ports.CDIWriter) *Writer {
	return &Writer{standard: standard, vfio: vfio}
}

// Write delegates CDI spec generation to the selected writer.
func (w *Writer) Write(ctx context.Context, req domain.PrepareRequest) (map[string][]string, error) {
	if w == nil {
		return nil, errors.New("CDI writer is nil")
	}
	if req.VFIO {
		if w.vfio == nil {
			return nil, errors.New("vfio CDI writer is not configured")
		}
		return w.vfio.Write(ctx, req)
	}
	if w.standard == nil {
		return nil, errors.New("standard CDI writer is not configured")
	}
	return w.standard.Write(ctx, req)
}

// Delete removes CDI specs from all configured writers.
func (w *Writer) Delete(ctx context.Context, claimUID string) error {
	if w == nil {
		return errors.New("CDI writer is nil")
	}
	var errs []error
	if w.standard != nil {
		if err := w.standard.Delete(ctx, claimUID); err != nil {
			errs = append(errs, err)
		}
	}
	if w.vfio != nil {
		if err := w.vfio.Delete(ctx, claimUID); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return fmt.Errorf("delete CDI specs: %v; %v", errs[0], errs[1])
}
