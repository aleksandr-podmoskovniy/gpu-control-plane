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

package gpuhandler

import "sync"

type featureGateTracker struct {
	mu       sync.Mutex
	disabled map[string]struct{}
}

func newFeatureGateTracker() *featureGateTracker {
	return &featureGateTracker{disabled: map[string]struct{}{}}
}

func (t *featureGateTracker) MarkDisabled(features []string) []string {
	if t == nil || len(features) == 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	var newly []string
	for _, feature := range features {
		if _, seen := t.disabled[feature]; seen {
			continue
		}
		t.disabled[feature] = struct{}{}
		newly = append(newly, feature)
	}
	return newly
}
