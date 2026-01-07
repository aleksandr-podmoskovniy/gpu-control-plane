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

package webhook

import (
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	resourcev1beta1 "k8s.io/api/resource/v1beta1"
	resourcev1beta2 "k8s.io/api/resource/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	resourceClaimResourceV1              = metav1.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceclaims"}
	resourceClaimTemplateResourceV1      = metav1.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceclaimtemplates"}
	resourceClaimResourceV1Beta1         = metav1.GroupVersionResource{Group: "resource.k8s.io", Version: "v1beta1", Resource: "resourceclaims"}
	resourceClaimTemplateResourceV1Beta1 = metav1.GroupVersionResource{Group: "resource.k8s.io", Version: "v1beta1", Resource: "resourceclaimtemplates"}
	resourceClaimResourceV1Beta2         = metav1.GroupVersionResource{Group: "resource.k8s.io", Version: "v1beta2", Resource: "resourceclaims"}
	resourceClaimTemplateResourceV1Beta2 = metav1.GroupVersionResource{Group: "resource.k8s.io", Version: "v1beta2", Resource: "resourceclaimtemplates"}
)

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

func init() {
	utilruntime.Must(resourcev1.AddToScheme(scheme))
	utilruntime.Must(resourcev1beta1.AddToScheme(scheme))
	utilruntime.Must(resourcev1beta2.AddToScheme(scheme))
}

func extractResourceClaim(req admission.Request) (*resourcev1.ResourceClaim, error) {
	raw := req.Object.Raw
	if len(raw) == 0 {
		return nil, fmt.Errorf("request object is empty")
	}

	var obj runtime.Object
	switch req.Resource {
	case resourceClaimResourceV1:
		obj = &resourcev1.ResourceClaim{}
	case resourceClaimResourceV1Beta1:
		obj = &resourcev1beta1.ResourceClaim{}
	case resourceClaimResourceV1Beta2:
		obj = &resourcev1beta2.ResourceClaim{}
	default:
		return nil, fmt.Errorf("unsupported resource version: %s", req.Resource)
	}

	if _, _, err := codecs.UniversalDeserializer().Decode(raw, nil, obj); err != nil {
		return nil, err
	}

	var v1Claim resourcev1.ResourceClaim
	if err := scheme.Convert(obj, &v1Claim, nil); err != nil {
		return nil, fmt.Errorf("failed to convert to v1: %w", err)
	}

	return &v1Claim, nil
}

func extractResourceClaimTemplate(req admission.Request) (*resourcev1.ResourceClaimTemplate, error) {
	raw := req.Object.Raw
	if len(raw) == 0 {
		return nil, fmt.Errorf("request object is empty")
	}

	var obj runtime.Object
	switch req.Resource {
	case resourceClaimTemplateResourceV1:
		obj = &resourcev1.ResourceClaimTemplate{}
	case resourceClaimTemplateResourceV1Beta1:
		obj = &resourcev1beta1.ResourceClaimTemplate{}
	case resourceClaimTemplateResourceV1Beta2:
		obj = &resourcev1beta2.ResourceClaimTemplate{}
	default:
		return nil, fmt.Errorf("unsupported resource version: %s", req.Resource)
	}

	if _, _, err := codecs.UniversalDeserializer().Decode(raw, nil, obj); err != nil {
		return nil, err
	}

	var v1Template resourcev1.ResourceClaimTemplate
	if err := scheme.Convert(obj, &v1Template, nil); err != nil {
		return nil, fmt.Errorf("failed to convert to v1: %w", err)
	}

	return &v1Template, nil
}
