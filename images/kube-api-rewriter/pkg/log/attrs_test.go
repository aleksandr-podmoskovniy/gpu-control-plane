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

package log

import (
	"errors"
	"strings"
	"testing"
)

func TestSlogErr(t *testing.T) {
	attr := SlogErr(errors.New("boom"))
	if attr.Key != "err" {
		t.Fatalf("unexpected key: %s", attr.Key)
	}
	if !strings.Contains(attr.Value.String(), "boom") {
		t.Fatalf("unexpected value: %s", attr.Value.String())
	}
}

func TestBodyDiff(t *testing.T) {
	attr := BodyDiff("diff")
	if attr.Key != BodyDiffKey || attr.Value.String() != "diff" {
		t.Fatalf("unexpected diff attr: %#v", attr)
	}
}

func TestBodyDump(t *testing.T) {
	attr := BodyDump("dump")
	if attr.Key != BodyDumpKey || attr.Value.String() != "dump" {
		t.Fatalf("unexpected dump attr: %#v", attr)
	}
}
