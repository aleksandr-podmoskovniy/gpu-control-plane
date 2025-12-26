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
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Conder returns a condition definition.
type Conder interface {
	Condition() metav1.Condition
}

// SetCondition adds or updates a condition in the slice.
func SetCondition(c Conder, conditions *[]metav1.Condition) {
	newCondition := c.Condition()
	if conditions == nil {
		return
	}
	existingCondition := FindStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		if newCondition.LastTransitionTime.IsZero() {
			newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
		*conditions = append(*conditions, newCondition)
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.Status = newCondition.Status
		if !newCondition.LastTransitionTime.IsZero() {
			existingCondition.LastTransitionTime = newCondition.LastTransitionTime
		} else {
			existingCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
	}

	if existingCondition.Reason != newCondition.Reason {
		existingCondition.Reason = newCondition.Reason
		if !newCondition.LastTransitionTime.IsZero() &&
			newCondition.LastTransitionTime.After(existingCondition.LastTransitionTime.Time) {
			existingCondition.LastTransitionTime = newCondition.LastTransitionTime
		} else {
			existingCondition.LastTransitionTime = metav1.NewTime(time.Now())
		}
	}

	if existingCondition.Message != newCondition.Message {
		existingCondition.Message = newCondition.Message
	}

	if existingCondition.ObservedGeneration != newCondition.ObservedGeneration {
		existingCondition.ObservedGeneration = newCondition.ObservedGeneration
	}
}

// FindStatusCondition returns a pointer to the condition with the given type.
func FindStatusCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
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

// LastTransitionTime sets the last transition time.
func (c *ConditionBuilder) LastTransitionTime(lastTransitionTime time.Time) *ConditionBuilder {
	c.lastTransitionTime = metav1.NewTime(lastTransitionTime)
	return c
}
