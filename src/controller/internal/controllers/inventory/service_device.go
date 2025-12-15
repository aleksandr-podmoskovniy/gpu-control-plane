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
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/api/gpu/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	cpmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/reconciler"
)

type DeviceService interface {
	Reconcile(ctx context.Context, node *corev1.Node, snapshot deviceSnapshot, nodeLabels map[string]string, managed bool, approval DeviceApprovalPolicy, detections nodeDetection) (*v1alpha1.GPUDevice, contracts.Result, error)
}

type deviceService struct {
	client   client.Client
	scheme   *runtimeScheme
	recorder eventRecorder
	handlers []contracts.InventoryHandler
}

func newDeviceService(c client.Client, scheme *runtimeScheme, recorder eventRecorder, handlers []contracts.InventoryHandler) DeviceService {
	return &deviceService{
		client:   c,
		scheme:   scheme,
		recorder: recorder,
		handlers: handlers,
	}
}

func (s *deviceService) Reconcile(ctx context.Context, node *corev1.Node, snapshot deviceSnapshot, nodeLabels map[string]string, managed bool, approval DeviceApprovalPolicy, detections nodeDetection) (*v1alpha1.GPUDevice, contracts.Result, error) {
	deviceName := buildDeviceName(node.Name, snapshot)
	device := &v1alpha1.GPUDevice{}
	err := s.client.Get(ctx, types.NamespacedName{Name: deviceName}, device)
	if apierrors.IsNotFound(err) {
		return s.createDevice(ctx, node, snapshot, nodeLabels, managed, approval, detections)
	}
	if err != nil {
		return nil, contracts.Result{}, err
	}

	metaUpdated, err := s.ensureDeviceMetadata(ctx, node, device, snapshot)
	if err != nil {
		return nil, contracts.Result{}, err
	}
	if metaUpdated {
		if err := s.client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
			return nil, contracts.Result{}, err
		}
	}

	statusBefore := device.DeepCopy()
	desiredInventoryID := buildInventoryID(node.Name, snapshot)

	if device.Status.NodeName != node.Name {
		device.Status.NodeName = node.Name
	}
	if device.Status.InventoryID != desiredInventoryID {
		device.Status.InventoryID = desiredInventoryID
	}
	if device.Status.Managed != managed {
		device.Status.Managed = managed
	}
	if device.Status.Hardware.PCI.Vendor != snapshot.Vendor ||
		device.Status.Hardware.PCI.Device != snapshot.Device ||
		device.Status.Hardware.PCI.Class != snapshot.Class ||
		device.Status.Hardware.PCI.Address != snapshot.PCIAddress {
		device.Status.Hardware.PCI.Vendor = snapshot.Vendor
		device.Status.Hardware.PCI.Device = snapshot.Device
		device.Status.Hardware.PCI.Class = snapshot.Class
		device.Status.Hardware.PCI.Address = snapshot.PCIAddress
	}
	if device.Status.Hardware.Product != snapshot.Product {
		device.Status.Hardware.Product = snapshot.Product
	}
	if device.Status.Hardware.UUID != snapshot.UUID {
		device.Status.Hardware.UUID = snapshot.UUID
	}
	if !equality.Semantic.DeepEqual(device.Status.Hardware.MIG, snapshot.MIG) {
		device.Status.Hardware.MIG = snapshot.MIG
	}
	autoAttach := approval.AutoAttach(managed, labelsForDevice(snapshot, nodeLabels))
	if device.Status.AutoAttach != autoAttach {
		device.Status.AutoAttach = autoAttach
	}

	applyDetection(device, snapshot, detections)

	result, err := s.invokeHandlers(ctx, device)
	if err != nil {
		return nil, result, err
	}

	if !equality.Semantic.DeepEqual(statusBefore.Status, device.Status) {
		if err := s.client.Status().Patch(ctx, device, client.MergeFrom(statusBefore)); err != nil {
			if apierrors.IsConflict(err) {
				return device, contracts.MergeResult(result, contracts.Result{Requeue: true}), nil
			}
			return nil, result, err
		}
	}

	return device, result, nil
}

func (s *deviceService) createDevice(ctx context.Context, node *corev1.Node, snapshot deviceSnapshot, nodeLabels map[string]string, managed bool, approval DeviceApprovalPolicy, detections nodeDetection) (*v1alpha1.GPUDevice, contracts.Result, error) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: buildDeviceName(node.Name, snapshot),
			Labels: map[string]string{
				deviceNodeLabelKey:  node.Name,
				deviceIndexLabelKey: snapshot.Index,
			},
		},
	}
	if err := controllerutil.SetOwnerReference(node, device, s.scheme); err != nil {
		return nil, contracts.Result{}, err
	}

	if err := s.client.Create(ctx, device); err != nil {
		return nil, contracts.Result{}, err
	}
	s.recorder.Eventf(device, corev1.EventTypeNormal, eventDeviceDetected, "Discovered GPU device index=%s vendor=%s device=%s on node %s", snapshot.Index, snapshot.Vendor, snapshot.Device, node.Name)

	device.Status.NodeName = node.Name
	device.Status.InventoryID = buildInventoryID(node.Name, snapshot)
	device.Status.Managed = managed
	device.Status.Hardware.PCI.Vendor = snapshot.Vendor
	device.Status.Hardware.PCI.Device = snapshot.Device
	device.Status.Hardware.PCI.Class = snapshot.Class
	device.Status.Hardware.PCI.Address = snapshot.PCIAddress
	device.Status.Hardware.Product = snapshot.Product
	device.Status.Hardware.UUID = snapshot.UUID
	device.Status.Hardware.MIG = snapshot.MIG
	device.Status.State = v1alpha1.GPUDeviceStateDiscovered
	device.Status.AutoAttach = approval.AutoAttach(managed, labelsForDevice(snapshot, nodeLabels))

	applyDetection(device, snapshot, detections)

	result, err := s.invokeHandlers(ctx, device)
	if err != nil {
		return nil, result, err
	}

	if err := s.client.Status().Update(ctx, device); err != nil {
		if apierrors.IsConflict(err) {
			return device, contracts.MergeResult(result, contracts.Result{Requeue: true}), nil
		}
		return nil, result, err
	}

	return device, result, nil
}

func (s *deviceService) ensureDeviceMetadata(ctx context.Context, node *corev1.Node, device *v1alpha1.GPUDevice, snapshot deviceSnapshot) (bool, error) {
	desired := device.DeepCopy()
	changed := false

	if desired.Labels == nil {
		desired.Labels = make(map[string]string)
	}
	if desired.Labels[deviceNodeLabelKey] != node.Name {
		desired.Labels[deviceNodeLabelKey] = node.Name
		changed = true
	}
	if desired.Labels[deviceIndexLabelKey] != snapshot.Index {
		desired.Labels[deviceIndexLabelKey] = snapshot.Index
		changed = true
	}
	if err := controllerutil.SetOwnerReference(node, desired, s.scheme); err != nil {
		return false, err
	}
	if !equality.Semantic.DeepEqual(device.GetOwnerReferences(), desired.GetOwnerReferences()) {
		changed = true
	}

	if !changed {
		return false, nil
	}

	if err := s.client.Patch(ctx, desired, client.MergeFrom(device)); err != nil {
		return false, err
	}
	*device = *desired

	return true, nil
}

func (s *deviceService) invokeHandlers(ctx context.Context, device *v1alpha1.GPUDevice) (contracts.Result, error) {
	rec := reconciler.NewBase(s.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler contracts.InventoryHandler) (contracts.Result, error) {
		result, err := handler.HandleDevice(ctx, device)
		if err != nil {
			cpmetrics.InventoryHandlerErrorInc(handler.Name())
		}
		return result, err
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	return rec.Reconcile(ctx)
}
