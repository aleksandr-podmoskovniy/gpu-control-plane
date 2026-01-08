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

package pciids

import "strings"

// Resolver provides name lookups for PCI IDs.
type Resolver struct {
	vendors   map[string]string
	devices   map[string]map[string]string
	classBase map[string]string
	classSub  map[string]map[string]string
}

// VendorName returns the vendor name for a vendor ID.
func (r *Resolver) VendorName(vendorID string) string {
	if r == nil {
		return ""
	}
	return r.vendors[strings.ToLower(vendorID)]
}

// DeviceName returns the device name for a vendor/device ID pair.
func (r *Resolver) DeviceName(vendorID, deviceID string) string {
	if r == nil {
		return ""
	}
	vendor := strings.ToLower(vendorID)
	device := strings.ToLower(deviceID)
	devices := r.devices[vendor]
	if devices == nil {
		return ""
	}
	return devices[device]
}

// ClassName returns the class name for a class code (base+subclass).
func (r *Resolver) ClassName(classCode string) string {
	if r == nil {
		return ""
	}
	code := strings.ToLower(classCode)
	if len(code) < 4 {
		return ""
	}
	base := code[:2]
	sub := code[2:4]
	if subs := r.classSub[base]; subs != nil {
		if name, ok := subs[sub]; ok {
			return name
		}
	}
	return r.classBase[base]
}
