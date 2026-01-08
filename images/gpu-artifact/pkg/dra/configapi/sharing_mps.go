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
	"errors"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/resource"
)

// ErrInvalidDeviceSelector indicates that a device index or UUID was invalid.
var ErrInvalidDeviceSelector error = errors.New("invalid device")

// ErrInvalidLimit indicates that a limit was invalid.
var ErrInvalidLimit error = errors.New("invalid limit")

// Normalize converts the per-device pinned memory limits into limits for the devices that are allocated.
func (m MpsPerDevicePinnedMemoryLimit) Normalize(uuids []string, defaultPinnedDeviceMemoryLimit *resource.Quantity) (map[string]string, error) {
	limits, err := (*limit)(defaultPinnedDeviceMemoryLimit).get(uuids)
	if err != nil {
		return nil, err
	}

	devices := newUUIDSet(uuids)
	for k, v := range m {
		id, err := devices.Normalize(k)
		if err != nil {
			return nil, err
		}
		megabyte, valid := (limit)(v).Megabyte()
		if !valid {
			return nil, fmt.Errorf("%w: value set too low: %v: %v", ErrInvalidLimit, k, v)
		}
		limits[id] = megabyte
	}
	return limits, nil
}

type limit resource.Quantity

func (d *limit) get(uuids []string) (map[string]string, error) {
	limits := make(map[string]string)
	if d == nil || len(uuids) == 0 {
		return limits, nil
	}

	megabyte, valid := d.Megabyte()
	if !valid {
		return nil, fmt.Errorf("%w: default value set too low: %v", ErrInvalidLimit, d)
	}
	for _, uuid := range uuids {
		limits[uuid] = megabyte
	}
	return limits, nil
}

func (d limit) Value() int64 {
	return (*resource.Quantity)(&d).Value()
}

func (d limit) Megabyte() (string, bool) {
	v := d.Value() / 1024 / 1024
	return fmt.Sprintf("%vM", v), v > 0
}

type uuidSet struct {
	uuids  []string
	lookup map[string]bool
}

// newUUIDSet creates a set of UUIDs for managing pinned memory for requested devices.
func newUUIDSet(uuids []string) *uuidSet {
	lookup := make(map[string]bool, len(uuids))
	for _, uuid := range uuids {
		lookup[uuid] = true
	}

	return &uuidSet{
		uuids:  uuids,
		lookup: lookup,
	}
}

func (s *uuidSet) Normalize(key string) (string, error) {
	if _, ok := s.lookup[key]; ok {
		return key, nil
	}

	index, err := strconv.Atoi(key)
	if err != nil {
		return "", fmt.Errorf("%w: unable to parse key as an integer: %v", ErrInvalidDeviceSelector, key)
	}
	if index >= 0 && index < len(s.uuids) {
		return s.uuids[index], nil
	}
	return "", fmt.Errorf("%w: invalid device index: %v", ErrInvalidDeviceSelector, index)
}
