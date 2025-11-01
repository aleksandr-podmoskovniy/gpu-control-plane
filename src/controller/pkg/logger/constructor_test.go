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

package logger

import (
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type recordingSink struct {
	values []interface{}
}

func (s *recordingSink) Init(logr.RuntimeInfo) {}

func (s *recordingSink) Enabled(int) bool { return true }

func (s *recordingSink) Info(int, string, ...interface{})    {}
func (s *recordingSink) Error(error, string, ...interface{}) {}

func (s *recordingSink) WithValues(kv ...interface{}) logr.LogSink {
	cp := &recordingSink{values: append(append([]interface{}{}, s.values...), kv...)}
	return cp
}

func (s *recordingSink) WithName(string) logr.LogSink {
	return &recordingSink{values: append([]interface{}{}, s.values...)}
}

func TestNewConstructorReturnsBaseLoggerForNilRequest(t *testing.T) {
	sink := &recordingSink{}
	base := logr.New(sink)

	constructor := NewConstructor(base)
	returned := constructor(nil)

	if returned.GetSink() != sink {
		t.Fatalf("expected base sink to be reused for nil request")
	}
}

func TestNewConstructorAppendsNamespacedValues(t *testing.T) {
	sink := &recordingSink{}
	base := logr.New(sink)

	constructor := NewConstructor(base)
	req := &reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "gpu-system", Name: "node-1"}}

	derived := constructor(req)
	derivedSink, ok := derived.GetSink().(*recordingSink)
	if !ok {
		t.Fatalf("expected recordingSink, got %T", derived.GetSink())
	}

	want := []interface{}{"namespace", "gpu-system", "name", "node-1"}
	if !reflect.DeepEqual(want, derivedSink.values) {
		t.Fatalf("unexpected values: want %v, got %v", want, derivedSink.values)
	}

	if derived.GetSink() == sink {
		t.Fatalf("expected a new sink instance for non-nil request")
	}
}
