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

package eventrecord

import (
	"fmt"
	"strings"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	eventTypeLabel         = "eventType"
	reasonLabel            = "reason"
	involvedNameLabel      = "involvedName"
	involvedNamespaceLabel = "involvedNamespace"
	involvedKindLabel      = "involvedKind"
)

// InfoLogger is a minimal interface used for event logging.
type InfoLogger interface {
	Info(msg string, args ...any)
}

type recorderProducer interface {
	GetEventRecorderFor(name string) record.EventRecorder
}

// EventRecorderLogger wraps client-go EventRecorder with logging.
type EventRecorderLogger interface {
	Event(object client.Object, eventtype, reason, message string)
	Eventf(involved client.Object, eventtype, reason, messageFmt string, args ...interface{})
	AnnotatedEventf(involved client.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{})
	WithLogging(logger InfoLogger) EventRecorderLogger
}

// NewEventRecorderLogger creates a recorder wrapper for a controller.
func NewEventRecorderLogger(recorderProducer recorderProducer, controllerName string) EventRecorderLogger {
	return &EventRecorderLoggerImpl{
		recorderProducer: recorderProducer,
		controllerName:   controllerName,
	}
}

// EventRecorderLoggerImpl implements EventRecorderLogger.
type EventRecorderLoggerImpl struct {
	controllerName   string
	recorderProducer recorderProducer
	logger           InfoLogger
}

// WithLogging returns a recorder that also logs events.
func (e *EventRecorderLoggerImpl) WithLogging(logger InfoLogger) EventRecorderLogger {
	return &EventRecorderLoggerImpl{
		controllerName:   e.controllerName,
		recorderProducer: e.recorderProducer,
		logger:           logger,
	}
}

// Event logs and records an event.
func (e *EventRecorderLoggerImpl) Event(object client.Object, eventtype, reason, message string) {
	e.logf(object, eventtype, reason, message, nil)
	recorder := e.recorderProducer.GetEventRecorderFor(e.recorderKey(reason))
	recorder.Event(object, eventtype, reason, message)
}

// Eventf logs and records a formatted event.
func (e *EventRecorderLoggerImpl) Eventf(object client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	e.logf(object, eventtype, reason, messageFmt, args...)
	recorder := e.recorderProducer.GetEventRecorderFor(e.recorderKey(reason))
	recorder.Eventf(object, eventtype, reason, messageFmt, args...)
}

// AnnotatedEventf logs and records a formatted event with annotations.
func (e *EventRecorderLoggerImpl) AnnotatedEventf(involved client.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	e.logf(involved, eventtype, reason, messageFmt, args...)
	recorder := e.recorderProducer.GetEventRecorderFor(e.recorderKey(reason))
	recorder.AnnotatedEventf(involved, annotations, eventtype, reason, messageFmt, args...)
}

func (e *EventRecorderLoggerImpl) logf(involved client.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	if e.logger == nil {
		return
	}
	e.logger.Info(
		fmt.Sprintf(messageFmt, args...),
		eventTypeLabel, eventtype,
		reasonLabel, reason,
		involvedNameLabel, involved.GetName(),
		involvedNamespaceLabel, involved.GetNamespace(),
		involvedKindLabel, involved.GetObjectKind().GroupVersionKind().Kind,
	)
}

func (e *EventRecorderLoggerImpl) recorderKey(reason string) string {
	return strings.Join([]string{e.controllerName, "/", reason}, "")
}
