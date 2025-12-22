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

package selection

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type selectionStatusPatchErrorWriter struct {
	client.StatusWriter
	err error
}

func (w selectionStatusPatchErrorWriter) Patch(context.Context, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
	return w.err
}

type selectionStatusPatchErrorClient struct {
	client.Client
	err error
}

func (c selectionStatusPatchErrorClient) Status() client.StatusWriter {
	return selectionStatusPatchErrorWriter{StatusWriter: c.Client.Status(), err: c.err}
}

type selectionGetErrorClient struct {
	client.Client
	err error
}

func (c selectionGetErrorClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return c.err
}
