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

package allocator

import (
	"context"
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"k8s.io/dynamic-resource-allocation/cel"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/k8s/allocatable"
	domainalloc "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/domain/allocatable"
	domainallocator "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/allocator"
)

type celSelector struct {
	expr    string
	program cel.CompilationResult
}

func (s celSelector) Match(ctx context.Context, driver string, spec domainalloc.DeviceSpec) (bool, error) {
	allow := spec.AllowMultipleAllocations
	input := cel.Device{
		Driver:                   driver,
		AllowMultipleAllocations: &allow,
		Attributes:               allocatable.RenderAttributes(spec.Attributes),
		Capacity:                 allocatable.RenderCapacities(spec.Capacity),
	}

	ok, _, err := s.program.DeviceMatches(ctx, input)
	if err != nil {
		return false, fmt.Errorf("selector %q failed: %w", s.expr, err)
	}
	return ok, nil
}

func compileSelectors(selectors []resourcev1.DeviceSelector) ([]domainallocator.Selector, error) {
	if len(selectors) == 0 {
		return nil, nil
	}

	compiler := cel.GetCompiler(cel.Features{EnableConsumableCapacity: true})
	out := make([]domainallocator.Selector, 0, len(selectors))

	for _, sel := range selectors {
		if sel.CEL == nil || sel.CEL.Expression == "" {
			return nil, fmt.Errorf("only CEL selectors are supported")
		}
		result := compiler.CompileCELExpression(sel.CEL.Expression, cel.Options{})
		if result.Error != nil {
			return nil, fmt.Errorf("invalid selector %q: %v", sel.CEL.Expression, result.Error)
		}
		out = append(out, celSelector{
			expr:    sel.CEL.Expression,
			program: result,
		})
	}
	return out, nil
}
