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

package service

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	invpci "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/pci"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
	invmetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/monitoring/metrics/inventory"
)

type DeviceHandler interface {
	HandleDevice(ctx context.Context, device *v1alpha1.GPUDevice) (reconcile.Result, error)
	Name() string
}

type DeviceService struct {
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	handlers []DeviceHandler
}

func NewDeviceService(c client.Client, scheme *runtime.Scheme, recorder record.EventRecorder, handlers []DeviceHandler) *DeviceService {
	return &DeviceService{
		client:   c,
		scheme:   scheme,
		recorder: recorder,
		handlers: handlers,
	}
}

func (s *DeviceService) Reconcile(
	ctx context.Context,
	node *corev1.Node,
	snapshot invstate.DeviceSnapshot,
	nodeLabels map[string]string,
	managed bool,
	approval invstate.DeviceApprovalPolicy,
	applyDetection func(*v1alpha1.GPUDevice, invstate.DeviceSnapshot),
) (*v1alpha1.GPUDevice, reconcile.Result, error) {
	deviceName := invstate.BuildDeviceName(node.Name, snapshot)
	device := &v1alpha1.GPUDevice{}
	device, err := commonobject.FetchObject(ctx, types.NamespacedName{Name: deviceName}, s.client, device)
	if err != nil {
		return nil, reconcile.Result{}, err
	}
	if device == nil {
		return s.createDevice(ctx, node, snapshot, nodeLabels, managed, approval, applyDetection)
	}

	metaUpdated, err := s.ensureDeviceMetadata(ctx, node, device, snapshot)
	if err != nil {
		return nil, reconcile.Result{}, err
	}
	if metaUpdated {
		if err := s.client.Get(ctx, types.NamespacedName{Name: deviceName}, device); err != nil {
			return nil, reconcile.Result{}, err
		}
	}

	statusBefore := device.DeepCopy()
	desiredInventoryID := invstate.BuildInventoryID(node.Name, snapshot)
	desiredPCIAddress := invpci.CanonicalizePCIAddress(snapshot.PCIAddress)

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
		device.Status.Hardware.PCI.Address != desiredPCIAddress {
		device.Status.Hardware.PCI.Vendor = snapshot.Vendor
		device.Status.Hardware.PCI.Device = snapshot.Device
		device.Status.Hardware.PCI.Class = snapshot.Class
		device.Status.Hardware.PCI.Address = desiredPCIAddress
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
	autoAttach := approval.AutoAttach(managed, invstate.LabelsForDevice(snapshot, nodeLabels))
	if device.Status.AutoAttach != autoAttach {
		device.Status.AutoAttach = autoAttach
	}

	if applyDetection != nil {
		applyDetection(device, snapshot)
	}
	device.Status.Hardware.PCI.Address = invpci.CanonicalizePCIAddress(device.Status.Hardware.PCI.Address)

	result, err := s.invokeHandlers(ctx, device)
	if err != nil {
		return nil, result, err
	}

	if !equality.Semantic.DeepEqual(statusBefore.Status, device.Status) {
		if err := s.client.Status().Patch(ctx, device, client.MergeFrom(statusBefore)); err != nil {
			if apierrors.IsConflict(err) {
				return device, reconciler.MergeResults(result, reconcile.Result{Requeue: true}), nil
			}
			return nil, result, err
		}
	}

	return device, result, nil
}

func (s *DeviceService) createDevice(
	ctx context.Context,
	node *corev1.Node,
	snapshot invstate.DeviceSnapshot,
	nodeLabels map[string]string,
	managed bool,
	approval invstate.DeviceApprovalPolicy,
	applyDetection func(*v1alpha1.GPUDevice, invstate.DeviceSnapshot),
) (*v1alpha1.GPUDevice, reconcile.Result, error) {
	device := &v1alpha1.GPUDevice{
		ObjectMeta: metav1.ObjectMeta{
			Name: invstate.BuildDeviceName(node.Name, snapshot),
			Labels: map[string]string{
				invstate.DeviceNodeLabelKey:  node.Name,
				invstate.DeviceIndexLabelKey: snapshot.Index,
			},
		},
	}
	if err := controllerutil.SetOwnerReference(node, device, s.scheme); err != nil {
		return nil, reconcile.Result{}, err
	}

	if err := s.client.Create(ctx, device); err != nil {
		return nil, reconcile.Result{}, err
	}
	if s.recorder != nil {
		s.recorder.Eventf(
			device,
			corev1.EventTypeNormal,
			invstate.EventDeviceDetected,
			"Discovered GPU device index=%s vendor=%s device=%s on node %s",
			snapshot.Index,
			snapshot.Vendor,
			snapshot.Device,
			node.Name,
		)
	}

	device.Status.NodeName = node.Name
	device.Status.InventoryID = invstate.BuildInventoryID(node.Name, snapshot)
	device.Status.Managed = managed
	device.Status.Hardware.PCI.Vendor = snapshot.Vendor
	device.Status.Hardware.PCI.Device = snapshot.Device
	device.Status.Hardware.PCI.Class = snapshot.Class
	device.Status.Hardware.PCI.Address = invpci.CanonicalizePCIAddress(snapshot.PCIAddress)
	device.Status.Hardware.Product = snapshot.Product
	device.Status.Hardware.UUID = snapshot.UUID
	device.Status.Hardware.MIG = snapshot.MIG
	device.Status.State = v1alpha1.GPUDeviceStateDiscovered
	device.Status.AutoAttach = approval.AutoAttach(managed, invstate.LabelsForDevice(snapshot, nodeLabels))

	if applyDetection != nil {
		applyDetection(device, snapshot)
	}
	device.Status.Hardware.PCI.Address = invpci.CanonicalizePCIAddress(device.Status.Hardware.PCI.Address)

	result, err := s.invokeHandlers(ctx, device)
	if err != nil {
		return nil, result, err
	}

	if err := s.client.Status().Update(ctx, device); err != nil {
		if apierrors.IsConflict(err) {
			return device, reconciler.MergeResults(result, reconcile.Result{Requeue: true}), nil
		}
		return nil, result, err
	}

	return device, result, nil
}

func (s *DeviceService) ensureDeviceMetadata(ctx context.Context, node *corev1.Node, device *v1alpha1.GPUDevice, snapshot invstate.DeviceSnapshot) (bool, error) {
	desired := device.DeepCopy()
	changed := false

	if desired.Labels == nil {
		desired.Labels = make(map[string]string)
	}
	if desired.Labels[invstate.DeviceNodeLabelKey] != node.Name {
		desired.Labels[invstate.DeviceNodeLabelKey] = node.Name
		changed = true
	}
	if desired.Labels[invstate.DeviceIndexLabelKey] != snapshot.Index {
		desired.Labels[invstate.DeviceIndexLabelKey] = snapshot.Index
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

func (s *DeviceService) invokeHandlers(ctx context.Context, device *v1alpha1.GPUDevice) (reconcile.Result, error) {
	rec := reconciler.NewBaseReconciler(s.handlers)
	rec.SetHandlerExecutor(func(ctx context.Context, handler DeviceHandler) (reconcile.Result, error) {
		result, err := handler.HandleDevice(ctx, device)
		if err != nil {
			invmetrics.InventoryHandlerErrorInc(handler.Name())
		}
		return result, err
	})
	rec.SetResourceUpdater(func(context.Context) error { return nil })

	return rec.Reconcile(ctx)
}
