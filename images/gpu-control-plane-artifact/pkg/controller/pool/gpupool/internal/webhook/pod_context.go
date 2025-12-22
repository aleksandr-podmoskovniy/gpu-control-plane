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

package webhook

import (
	"context"
	"strings"

	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func effectiveNamespace(ctx context.Context, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if req, err := cradmission.RequestFromContext(ctx); err == nil {
		return req.Namespace
	}
	return ""
}
