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

package configapi

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

const (
	GroupName = "resource.gpu.deckhouse.io"
	Version   = "v1alpha1"

	VfioDeviceConfigKind = "VfioDeviceConfig"
)

// Interface defines the common API for DRA configuration objects.
// +k8s:deepcopy-gen=false
type Interface interface {
	Normalize() error
	Validate() error
}

// StrictDecoder rejects unknown fields in user-provided configuration.
var StrictDecoder runtime.Decoder

// NonstrictDecoder ignores unknown fields (useful for checkpoints).
var NonstrictDecoder runtime.Decoder

func init() {
	scheme := runtime.NewScheme()
	schemeGroupVersion := schema.GroupVersion{
		Group:   GroupName,
		Version: Version,
	}
	scheme.AddKnownTypes(schemeGroupVersion,
		&VfioDeviceConfig{},
	)
	metav1.AddToGroupVersion(scheme, schemeGroupVersion)

	NonstrictDecoder = json.NewSerializerWithOptions(
		json.DefaultMetaFactory,
		scheme,
		scheme,
		json.SerializerOptions{Strict: false},
	)
	StrictDecoder = json.NewSerializerWithOptions(
		json.DefaultMetaFactory,
		scheme,
		scheme,
		json.SerializerOptions{Strict: true},
	)
}
