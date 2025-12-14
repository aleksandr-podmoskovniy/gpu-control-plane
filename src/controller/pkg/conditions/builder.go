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

package conditions

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Builder updates a slice of metav1.Conditions preserving transition times.
type Builder struct {
	target *[]metav1.Condition
}

// New constructs a Builder that operates on the provided slice pointer.
func New(target *[]metav1.Condition) Builder {
	return Builder{target: target}
}

// Set inserts or replaces a condition. LastTransitionTime is preserved when status is unchanged.
func (b Builder) Set(cond metav1.Condition) {
	if b.target == nil {
		return
	}

	cond.LastTransitionTime = metav1.Now()
	for i := range *b.target {
		existing := &(*b.target)[i]
		if existing.Type != cond.Type {
			continue
		}
		if existing.Status == cond.Status {
			cond.LastTransitionTime = existing.LastTransitionTime
		}
		(*b.target)[i] = cond
		return
	}

	*b.target = append(*b.target, cond)
}

// Find returns a pointer to the first condition matching the type.
func (b Builder) Find(conditionType string) *metav1.Condition {
	if b.target == nil {
		return nil
	}
	for i := range *b.target {
		if (*b.target)[i].Type == conditionType {
			return &(*b.target)[i]
		}
	}
	return nil
}
