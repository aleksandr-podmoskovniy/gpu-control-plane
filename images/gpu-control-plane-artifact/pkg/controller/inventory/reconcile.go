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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/aleksandr-podmoskovniy/gpu-control-plane/api/gpu/v1alpha1"
	commonobject "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/common/object"
	"github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/contracts"
	invconsts "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/consts"
	invservice "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/service"
	invstate "github.com/aleksandr-podmoskovniy/gpu-control-plane/controller/pkg/controller/inventory/internal/state"
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", req.Name)
	ctx = logr.NewContext(ctx, logger)

	managedPolicy, approvalPolicy := r.currentPolicies()

	node := &corev1.Node{}
	node, err := commonobject.FetchObject(ctx, req.NamespacedName, r.client, node)
	if err != nil {
		return ctrl.Result{}, err
	}
	if node == nil {
		// Rely on ownerReferences GC; avoid aggressive cleanup that may fire on transient cache misses.
		logger.V(1).Info("node removed, skipping reconciliation")
		r.cleanupSvc().ClearMetrics(req.Name)
		return ctrl.Result{}, nil
	}

	nodeFeature, err := r.findNodeFeature(ctx, node.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	state := newInventoryState(node, nodeFeature, managedPolicy)
	nodeSnapshot := state.Snapshot()
	snapshotList := nodeSnapshot.Devices
	managed := nodeSnapshot.Managed

	// If NodeFeature data has not arrived yet and we have no device snapshots,
	// avoid deleting existing GPUDevice/GPUNodeState. We'll be requeued by the
	// NodeFeature watch once it appears.
	if !nodeSnapshot.FeatureDetected && len(snapshotList) == 0 {
		logger.V(1).Info("node feature not detected yet, skip reconcile")
		return ctrl.Result{}, nil
	}

	allowCleanup := state.AllowCleanup()
	var orphanDevices map[string]struct{}
	if allowCleanup {
		if orphanDevices, err = state.OrphanDevices(ctx, r.client); err != nil {
			return ctrl.Result{}, err
		}
	}

	reconciledDevices := make([]*v1alpha1.GPUDevice, 0, len(snapshotList))
	aggregate := contracts.Result{}

	var detections invservice.NodeDetection
	if d, err := state.CollectDetections(ctx, r.collectNodeDetections); err == nil {
		detections = d
	} else {
		logger.V(1).Info("gfd-extender telemetry unavailable", "node", node.Name, "error", err)
		r.recorder.Eventf(node, corev1.EventTypeWarning, invconsts.EventDetectUnavailable, "gfd-extender unavailable for node %s: %v", node.Name, err)
	}

	for _, snapshot := range snapshotList {
		device, res, err := r.deviceSvc().Reconcile(ctx, node, snapshot, nodeSnapshot.Labels, managed, approvalPolicy, func(device *v1alpha1.GPUDevice, snapshot invstate.DeviceSnapshot) {
			invservice.ApplyDetection(device, snapshot, detections)
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		if orphanDevices != nil {
			delete(orphanDevices, device.Name)
		}
		reconciledDevices = append(reconciledDevices, device)
		aggregate = contracts.MergeResult(aggregate, res)
	}

	if node.GetDeletionTimestamp() != nil {
		if err := r.cleanupSvc().RemoveOrphans(ctx, node, orphanDevices); err != nil {
			return ctrl.Result{}, err
		}
	}

	ctrlResult := ctrl.Result{}
	if err := r.inventorySvc().Reconcile(ctx, node, nodeSnapshot, reconciledDevices); err != nil {
		if apierrors.IsConflict(err) {
			// GPUNodeState.status is updated by multiple controllers; conflicts are expected and handled by requeue.
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}
	r.inventorySvc().UpdateDeviceMetrics(node.Name, reconciledDevices)

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
		logger.V(1).Info("inventory reconcile scheduled follow-up", "requeue", ctrlResult.Requeue, "after", ctrlResult.RequeueAfter)
	} else {
		logger.V(2).Info("inventory reconcile completed", "devices", len(reconciledDevices))
	}

	return ctrlResult, nil
}
