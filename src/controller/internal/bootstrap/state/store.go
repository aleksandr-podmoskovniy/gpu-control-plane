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

package state

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	currentTime      = func() metav1.Time { return metav1.Now() }
	marshalNodeState = func(state NodeState) ([]byte, error) { return yaml.Marshal(state) }
)

const (
	nodeDataSuffix      = ".yaml"
	placeholderHostname = "__bootstrap-disabled__"
)

// NodeState persists bootstrap phase flags for a node.
type NodeState struct {
	Phase      string          `json:"phase"`
	Components map[string]bool `json:"components"`
	UpdatedAt  metav1.Time     `json:"updatedAt"`
}

// Store persists bootstrap state in a ConfigMap so that Helm templates can read it.
type Store struct {
	client    client.Client
	reader    client.Reader
	namespace string
	name      string
	owner     types.NamespacedName
}

// NewStore constructs a bootstrap state store writing to the given ConfigMap.
func NewStore(cl client.Client, reader client.Reader, namespace, name string, owner types.NamespacedName) *Store {
	return &Store{
		client:    cl,
		reader:    reader,
		namespace: namespace,
		name:      name,
		owner:     owner,
	}
}

// Ensure guarantees that the ConfigMap exists.
func (s *Store) Ensure(ctx context.Context) error {
	cm := &corev1.ConfigMap{}
	err := s.getReader().Get(ctx, types.NamespacedName{Name: s.name, Namespace: s.namespace}, cm)
	switch {
	case err == nil:
		return nil
	case !apierrors.IsNotFound(err):
		return err
	}

	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      s.name,
			Namespace: s.namespace,
		},
		Data: map[string]string{},
	}
	if err := s.setOwnerReference(ctx, cm); err != nil {
		return err
	}
	return s.client.Create(ctx, cm)
}

// UpdateNode stores the state entry for the node.
func (s *Store) UpdateNode(ctx context.Context, node string, state NodeState) error {
	state = normaliseNodeState(state)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm := &corev1.ConfigMap{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: s.name, Namespace: s.namespace}, cm); err != nil {
			if apierrors.IsNotFound(err) {
				if err := s.Ensure(ctx); err != nil {
					return err
				}
				return fmt.Errorf("bootstrap state configmap created, retry update")
			}
			return err
		} else {
			_ = cm
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		payload, err := marshalNodeState(state)
		if err != nil {
			return fmt.Errorf("marshal node state: %w", err)
		}
		key := nodeKey(node)
		if existing, ok := cm.Data[key]; ok {
			same, err := compareStatePayload(existing, state)
			if err != nil {
				return err
			}
			if same {
				return nil
			}
		}
		cm.Data[key] = string(payload)
		return s.client.Update(ctx, cm)
	})
}

// DeleteNode removes cached state for the node.
func (s *Store) DeleteNode(ctx context.Context, node string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		cm := &corev1.ConfigMap{}
		if err := s.client.Get(ctx, types.NamespacedName{Name: s.name, Namespace: s.namespace}, cm); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		if cm.Data == nil {
			return nil
		}
		key := nodeKey(node)
		if _, ok := cm.Data[key]; !ok {
			return nil
		}
		delete(cm.Data, key)
		return s.client.Update(ctx, cm)
	})
}

func (s *Store) setOwnerReference(ctx context.Context, cm *corev1.ConfigMap) error {
	if s.owner.Name == "" || s.owner.Namespace == "" {
		return nil
	}
	deploy := &appsv1.Deployment{}
	if err := s.getReader().Get(ctx, s.owner, deploy); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			return nil
		}
		return err
	}
	cm.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: appsv1.SchemeGroupVersion.String(),
			Kind:       "Deployment",
			Name:       deploy.Name,
			UID:        deploy.UID,
		},
	}
	return nil
}

func nodeKey(node string) string {
	return fmt.Sprintf("%s%s", node, nodeDataSuffix)
}

func compareStatePayload(existing string, desired NodeState) (bool, error) {
	var current NodeState
	if err := yaml.Unmarshal([]byte(existing), &current); err != nil {
		return false, fmt.Errorf("unmarshal existing state: %w", err)
	}
	return current.Phase == desired.Phase && reflect.DeepEqual(current.Components, desired.Components), nil
}

func normaliseNodeState(state NodeState) NodeState {
	state.UpdatedAt = currentTime()
	if state.Components == nil {
		state.Components = map[string]bool{}
	}
	return state
}

func (s *Store) getReader() client.Reader {
	if s.reader != nil {
		return s.reader
	}
	return s.client
}
