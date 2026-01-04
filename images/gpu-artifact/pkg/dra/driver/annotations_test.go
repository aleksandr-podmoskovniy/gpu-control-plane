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

package driver

import "testing"

func TestVFIORequested(t *testing.T) {
	cases := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{name: "empty", annotations: nil, expected: false},
		{name: "missing", annotations: map[string]string{"x": "y"}, expected: false},
		{name: "false", annotations: map[string]string{VfioAnnotationKey: "false"}, expected: false},
		{name: "true", annotations: map[string]string{VfioAnnotationKey: "true"}, expected: true},
		{name: "true-case", annotations: map[string]string{VfioAnnotationKey: "TrUe"}, expected: true},
		{name: "yes", annotations: map[string]string{VfioAnnotationKey: "yes"}, expected: true},
		{name: "one", annotations: map[string]string{VfioAnnotationKey: "1"}, expected: true},
	}

	for _, tc := range cases {
		if got := VFIORequested(tc.annotations); got != tc.expected {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.expected, got)
		}
	}
}
