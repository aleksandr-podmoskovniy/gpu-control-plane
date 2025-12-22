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

package handler

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	invservice "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/service"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
	ctrlreconciler "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/reconciler"
)

// InventoryHandler reconciles GPUDevice and GPUNodeState resources for a node.
type InventoryHandler struct {
	log          logr.Logger
	client       client.Client
	deviceSvc    DeviceService
	inventorySvc InventoryService
	cleanupSvc   CleanupService
	detectionSvc DetectionCollector
	recorder     record.EventRecorder
}

func NewInventoryHandler(
	log logr.Logger,
	client client.Client,
	deviceSvc DeviceService,
	inventorySvc InventoryService,
	cleanupSvc CleanupService,
	detectionSvc DetectionCollector,
	recorder record.EventRecorder,
) *InventoryHandler {
	return &InventoryHandler{
		log:          log,
		client:       client,
		deviceSvc:    deviceSvc,
		inventorySvc: inventorySvc,
		cleanupSvc:   cleanupSvc,
		detectionSvc: detectionSvc,
		recorder:     recorder,
	}
}

func (h *InventoryHandler) Name() string {
	return "inventory"
}

func (h *InventoryHandler) Handle(ctx context.Context, state invstate.InventoryState) (reconcile.Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	node := state.Node()
	if node == nil {
		return reconcile.Result{}, nil
	}

	nodeSnapshot := state.Snapshot()
	snapshotList := nodeSnapshot.Devices

	if !nodeSnapshot.FeatureDetected && len(snapshotList) == 0 {
		log.V(1).Info("node feature not detected yet, skip reconcile")
		return reconcile.Result{}, nil
	}

	var orphanDevices map[string]struct{}
	if state.AllowCleanup() {
		var err error
		orphanDevices, err = state.OrphanDevices(ctx, h.client)
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	reconciledDevices := make([]*v1alpha1.GPUDevice, 0, len(snapshotList))
	aggregate := reconcile.Result{}

	var detections invservice.NodeDetection
	if state.HasDevices() {
		if d, err := h.detectionSvc.Collect(ctx, node.Name); err == nil {
			detections = d
		} else {
			log.V(1).Info("gfd-extender telemetry unavailable", "node", node.Name, "error", err)
			if h.recorder != nil {
				h.recorder.Eventf(node, corev1.EventTypeWarning, invstate.EventDetectUnavailable, "gfd-extender unavailable for node %s: %v", node.Name, err)
			}
		}
	}

	for _, snapshot := range snapshotList {
		device, res, err := h.deviceSvc.Reconcile(ctx, node, snapshot, nodeSnapshot.Labels, nodeSnapshot.Managed, state.ApprovalPolicy(), func(device *v1alpha1.GPUDevice, snapshot invstate.DeviceSnapshot) {
			invservice.ApplyDetection(device, snapshot, detections)
		})
		if err != nil {
			return reconcile.Result{}, err
		}
		if orphanDevices != nil && device != nil {
			delete(orphanDevices, device.Name)
		}
		reconciledDevices = append(reconciledDevices, device)
		aggregate = ctrlreconciler.MergeResults(aggregate, res)
	}

	if node.GetDeletionTimestamp() != nil {
		if err := h.cleanupSvc.RemoveOrphans(ctx, node, orphanDevices); err != nil {
			return reconcile.Result{}, err
		}
	}

	if err := h.inventorySvc.Reconcile(ctx, node, nodeSnapshot, reconciledDevices); err != nil {
		if apierrors.IsConflict(err) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, err
	}
	h.inventorySvc.UpdateDeviceMetrics(node.Name, reconciledDevices)

	ctrlResult := reconcile.Result{}
	hasDevices := len(reconciledDevices) > 0
	if hasDevices && aggregate.Requeue {
		ctrlResult.Requeue = true
	}
	if hasDevices && aggregate.RequeueAfter > 0 {
		if ctrlResult.RequeueAfter == 0 || aggregate.RequeueAfter < ctrlResult.RequeueAfter {
			ctrlResult.RequeueAfter = aggregate.RequeueAfter
		}
	}

	if ctrlResult.Requeue || ctrlResult.RequeueAfter > 0 {
		log.V(1).Info("inventory reconcile scheduled follow-up", "requeue", ctrlResult.Requeue, "after", ctrlResult.RequeueAfter)
	} else {
		log.V(2).Info("inventory reconcile completed", "devices", len(reconciledDevices))
	}

	return ctrlResult, nil
}
