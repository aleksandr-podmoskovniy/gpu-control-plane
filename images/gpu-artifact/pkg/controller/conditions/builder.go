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

package conditions

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Conder returns a condition definition.
type Conder interface {
	Condition() metav1.Condition
}

// RemoveCondition removes a condition from the slice.
func RemoveCondition(conditionType Stringer, conditions *[]metav1.Condition) {
	meta.RemoveStatusCondition(conditions, conditionType.String())
}

// NewConditionBuilder creates a condition builder with defaults.
func NewConditionBuilder(conditionType Stringer) *ConditionBuilder {
	return &ConditionBuilder{
		status:        metav1.ConditionUnknown,
		reason:        ReasonUnknown.String(),
		conditionType: conditionType,
	}
}

// ConditionBuilder builds a metav1.Condition.
type ConditionBuilder struct {
	status             metav1.ConditionStatus
	conditionType      Stringer
	reason             string
	message            string
	generation         int64
	lastTransitionTime metav1.Time
}

// Condition returns the built condition.
func (c *ConditionBuilder) Condition() metav1.Condition {
	return metav1.Condition{
		Type:               c.conditionType.String(),
		Status:             c.status,
		Reason:             c.reason,
		Message:            c.message,
		ObservedGeneration: c.generation,
		LastTransitionTime: c.lastTransitionTime,
	}
}

// Status sets the condition status.
func (c *ConditionBuilder) Status(status metav1.ConditionStatus) *ConditionBuilder {
	if status != "" {
		c.status = status
	}
	return c
}

// Reason sets the condition reason.
func (c *ConditionBuilder) Reason(reason Stringer) *ConditionBuilder {
	if reason.String() != "" {
		c.reason = reason.String()
	}
	return c
}

// Message sets the condition message.
func (c *ConditionBuilder) Message(msg string) *ConditionBuilder {
	c.message = msg
	return c
}

// Generation sets the observed generation.
func (c *ConditionBuilder) Generation(generation int64) *ConditionBuilder {
	c.generation = generation
	return c
}
