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

package service

import (
	"context"
	"fmt"

	resourcev1 "k8s.io/api/resource/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeviceClassService loads DeviceClass objects for a claim.
type DeviceClassService struct {
	client client.Client
}

// NewDeviceClassService constructs a DeviceClassService.
func NewDeviceClassService(client client.Client) *DeviceClassService {
	return &DeviceClassService{client: client}
}

// Load returns DeviceClass objects referenced by the claim.
func (s *DeviceClassService) Load(ctx context.Context, claim *resourcev1.ResourceClaim) (map[string]*resourcev1.DeviceClass, error) {
	if claim == nil {
		return nil, nil
	}
	names := deviceClassNames(claim)
	if len(names) == 0 {
		return nil, nil
	}

	classes := make(map[string]*resourcev1.DeviceClass, len(names))
	for _, name := range names {
		obj := &resourcev1.DeviceClass{}
		if err := s.client.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
			return nil, fmt.Errorf("deviceclass %q: %w", name, err)
		}
		classes[name] = obj
	}
	return classes, nil
}

func deviceClassNames(claim *resourcev1.ResourceClaim) []string {
	seen := map[string]struct{}{}
	for _, req := range claim.Spec.Devices.Requests {
		if req.Exactly != nil && req.Exactly.DeviceClassName != "" {
			seen[req.Exactly.DeviceClassName] = struct{}{}
		}
		for _, sub := range req.FirstAvailable {
			if sub.DeviceClassName != "" {
				seen[sub.DeviceClassName] = struct{}{}
			}
		}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
}
