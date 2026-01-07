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

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare"
)

// Driver implements kubeletplugin.DRAPlugin and publishes ResourceSlices.
type Driver struct {
	helper         *kubeletplugin.Helper
	driverName     string
	nodeName       string
	kubeClient     kubernetes.Interface
	prepareService *prepare.Service
	errorHandler   func(ctx context.Context, err error, msg string)
}

// PublishResources publishes ResourceSlices for this driver.
func (d *Driver) PublishResources(ctx context.Context, resources resourceslice.DriverResources) error {
	if d.helper == nil {
		return errors.New("DRA plugin is not started")
	}
	return d.helper.PublishResources(ctx, resources)
}

// Shutdown stops the DRA plugin.
func (d *Driver) Shutdown() {
	if d.helper != nil {
		d.helper.Stop()
	}
}

// HandleError forwards background errors to the Kubernetes runtime error handler.
func (d *Driver) HandleError(ctx context.Context, err error, msg string) {
	if d.errorHandler != nil {
		d.errorHandler(ctx, err, msg)
		return
	}
	utilruntime.HandleErrorWithContext(ctx, err, msg)
}
