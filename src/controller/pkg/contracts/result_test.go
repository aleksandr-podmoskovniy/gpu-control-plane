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

package contracts

import (
	"testing"
	"time"
)

func TestMergeResult(t *testing.T) {
	cases := []struct {
		name   string
		first  Result
		second Result
		expect Result
	}{
		{
			name:   "second requeue overrides",
			first:  Result{},
			second: Result{Requeue: true},
			expect: Result{Requeue: true},
		},
		{
			name:   "shorter delay wins",
			first:  Result{RequeueAfter: 10 * time.Second},
			second: Result{RequeueAfter: 2 * time.Second},
			expect: Result{RequeueAfter: 2 * time.Second},
		},
		{
			name:   "longer delay ignored",
			first:  Result{RequeueAfter: 2 * time.Second},
			second: Result{RequeueAfter: 10 * time.Second},
			expect: Result{RequeueAfter: 2 * time.Second},
		},
	}

	for _, tc := range cases {
		if got := MergeResult(tc.first, tc.second); got != tc.expect {
			t.Fatalf("%s: expected %+v, got %+v", tc.name, tc.expect, got)
		}
	}
}
