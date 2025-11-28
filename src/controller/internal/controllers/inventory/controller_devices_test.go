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

package inventory

import (
	"testing"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
)

func TestIsDeviceIgnored(t *testing.T) {
	if isDeviceIgnored(nil) {
		t.Fatal("nil device should not be ignored")
	}
	dev := &v1alpha1.GPUDevice{}
	if isDeviceIgnored(dev) {
		t.Fatal("unexpected ignore for empty device")
	}
	dev.Annotations = map[string]string{deviceIgnoreAnnotation: "true"}
	if !isDeviceIgnored(dev) {
		t.Fatal("expected ignore via annotation")
	}
	dev.Annotations = nil
	dev.Labels = map[string]string{deviceIgnoreLabel: "true"}
	if !isDeviceIgnored(dev) {
		t.Fatal("expected ignore via label")
	}
}
