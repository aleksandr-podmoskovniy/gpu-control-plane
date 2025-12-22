// Copyright 2025 Flant JSC
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

package names

import (
	"fmt"
	"strings"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	poolcommon "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/service/pool/common"
)

// PoolResourceName returns the fully-qualified resource name used by the device plugin.
func PoolResourceName(pool *v1alpha1.GPUPool) string {
	return fmt.Sprintf("%s/%s", poolcommon.PoolResourcePrefixFor(pool), ResolveResourceName(pool, pool.Name))
}

// ResolveResourceName returns unqualified resource name (prefix stripped).
func ResolveResourceName(pool *v1alpha1.GPUPool, rawName string) string {
	name := strings.TrimSpace(rawName)
	if name == "" && pool != nil {
		name = pool.Name
	}

	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		if len(parts) > 1 {
			name = parts[len(parts)-1]
		}
	}

	return name
}
