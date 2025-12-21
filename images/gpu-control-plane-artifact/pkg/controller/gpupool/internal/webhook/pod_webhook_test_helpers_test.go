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

package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	admv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	cradmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/moduleconfig"
)

var errEmptyPodAdmissionRequest = errors.New("empty pod admission request")

type podMutatorAdapter struct {
	defaulter *PodDefaulter
}

func newPodMutator(log logr.Logger, store *moduleconfig.ModuleConfigStore, c client.Client) *podMutatorAdapter {
	return &podMutatorAdapter{
		defaulter: NewPodDefaulter(log, store, c),
	}
}

func (a *podMutatorAdapter) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	pod, original, err := decodePodAdmissionObject(req.Object)
	if err != nil {
		if errors.Is(err, errEmptyPodAdmissionRequest) {
			return cradmission.Denied(err.Error())
		}
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	ctx = cradmission.NewContextWithRequest(ctx, req)
	if err := a.defaulter.Default(ctx, pod); err != nil {
		return cradmission.Denied(err.Error())
	}

	mutated, err := json.Marshal(pod)
	if err != nil {
		return cradmission.Errored(http.StatusInternalServerError, fmt.Errorf("marshal mutated pod: %w", err))
	}
	return cradmission.PatchResponseFromRaw(original, mutated)
}

type podValidatorAdapter struct {
	validator *PodValidator
}

func newPodValidator(log logr.Logger, c client.Client) *podValidatorAdapter {
	return &podValidatorAdapter{
		validator: NewPodValidator(log, c),
	}
}

func (a *podValidatorAdapter) Handle(ctx context.Context, req cradmission.Request) cradmission.Response {
	target := req.Object
	if req.Operation == admv1.Delete {
		target = req.OldObject
	}

	pod, _, err := decodePodAdmissionObject(target)
	if err != nil {
		if errors.Is(err, errEmptyPodAdmissionRequest) {
			return cradmission.Denied(err.Error())
		}
		return cradmission.Errored(http.StatusUnprocessableEntity, err)
	}

	ctx = cradmission.NewContextWithRequest(ctx, req)
	warnings, err := a.validator.validate(ctx, pod)
	if err != nil {
		return cradmission.Denied(err.Error()).WithWarnings(warnings...)
	}
	return cradmission.Allowed("").WithWarnings(warnings...)
}

func decodePodAdmissionObject(obj runtime.RawExtension) (*corev1.Pod, []byte, error) {
	switch {
	case len(obj.Raw) > 0:
		pod := &corev1.Pod{}
		if err := json.Unmarshal(obj.Raw, pod); err != nil {
			return nil, nil, err
		}
		return pod, obj.Raw, nil
	case obj.Object != nil:
		pod, ok := obj.Object.(*corev1.Pod)
		if !ok {
			return nil, nil, fmt.Errorf("request object is not a Pod")
		}
		raw, err := json.Marshal(pod)
		if err != nil {
			return nil, nil, fmt.Errorf("marshal pod admission object: %w", err)
		}
		return pod, raw, nil
	default:
		return nil, nil, errEmptyPodAdmissionRequest
	}
}
