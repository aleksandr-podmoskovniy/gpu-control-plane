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

package patch

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	PatchReplaceOp = "replace"
	PatchAddOp     = "add"
	PatchRemoveOp  = "remove"
	PatchTestOp    = "test"
)

// JSONPatch represents a list of JSONPatch operations.
type JSONPatch struct {
	operations []JSONPatchOperation
}

// JSONPatchOperation is a single JSONPatch operation.
type JSONPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// NewJSONPatch creates a JSONPatch with optional operations.
func NewJSONPatch(patches ...JSONPatchOperation) *JSONPatch {
	return &JSONPatch{
		operations: patches,
	}
}

// NewJSONPatchOperation creates a JSONPatchOperation.
func NewJSONPatchOperation(op, path string, value interface{}) JSONPatchOperation {
	return JSONPatchOperation{
		Op:    op,
		Path:  path,
		Value: value,
	}
}

// WithAdd creates an add operation.
func WithAdd(path string, value interface{}) JSONPatchOperation {
	return NewJSONPatchOperation(PatchAddOp, path, value)
}

// WithRemove creates a remove operation.
func WithRemove(path string) JSONPatchOperation {
	return NewJSONPatchOperation(PatchRemoveOp, path, nil)
}

// WithReplace creates a replace operation.
func WithReplace(path string, value interface{}) JSONPatchOperation {
	return NewJSONPatchOperation(PatchReplaceOp, path, value)
}

// Operations returns the list of operations.
func (jp *JSONPatch) Operations() []JSONPatchOperation {
	return jp.operations
}

// Append adds operations to the patch.
func (jp *JSONPatch) Append(patches ...JSONPatchOperation) {
	jp.operations = append(jp.operations, patches...)
}

// Len returns the number of operations.
func (jp *JSONPatch) Len() int {
	return len(jp.operations)
}

// Bytes serializes the patch to JSON.
func (jp *JSONPatch) Bytes() ([]byte, error) {
	if jp.Len() == 0 {
		return nil, fmt.Errorf("list of patches is empty")
	}
	return json.Marshal(jp.operations)
}

// EscapeJSONPointer escapes a JSON Pointer token.
func EscapeJSONPointer(path string) string {
	path = strings.ReplaceAll(path, "~", "~0")
	path = strings.ReplaceAll(path, "/", "~1")
	return path
}
