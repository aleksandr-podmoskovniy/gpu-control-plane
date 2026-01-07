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

package driver

import (
	"context"
	"errors"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

func (d *Driver) listResourceSlices(ctx context.Context) ([]resourceapi.ResourceSlice, error) {
	if d == nil || d.kubeClient == nil {
		return nil, errors.New("kube client is required")
	}
	list, err := d.kubeClient.ResourceV1().ResourceSlices().List(ctx, resourceSliceListOptions(d.driverName, d.nodeName))
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func resourceSliceListOptions(driverName, nodeName string) metav1.ListOptions {
	selector := fields.Set{}
	if driverName != "" {
		selector[resourceapi.ResourceSliceSelectorDriver] = driverName
	}
	if nodeName != "" {
		selector[resourceapi.ResourceSliceSelectorNodeName] = nodeName
	}
	if len(selector) == 0 {
		return metav1.ListOptions{}
	}
	return metav1.ListOptions{FieldSelector: selector.String()}
}
