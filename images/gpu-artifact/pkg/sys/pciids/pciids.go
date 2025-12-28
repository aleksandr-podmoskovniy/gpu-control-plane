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

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Resolver provides name lookups for PCI IDs.
type Resolver struct {
	vendors   map[string]string
	devices   map[string]map[string]string
	classBase map[string]string
	classSub  map[string]map[string]string
}

// Load parses a pci.ids file.
func Load(path string) (*Resolver, error) {
	if path == "" {
		return nil, fmt.Errorf("pci.ids path is empty")
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	res := &Resolver{
		vendors:   map[string]string{},
		devices:   map[string]map[string]string{},
		classBase: map[string]string{},
		classSub:  map[string]map[string]string{},
	}

	scanner := bufio.NewScanner(file)
	mode := ""
	currentVendor := ""
	currentClass := ""

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		indentTabs, indentSpaces := leadingIndent(line)
		trimmed := strings.TrimLeft(line, " \t")
		if indentTabs == 0 && indentSpaces == 0 {
			currentVendor = ""
			currentClass = ""

			if strings.HasPrefix(trimmed, "C ") {
				mode = "class"
				fields := strings.Fields(trimmed)
				if len(fields) < 3 {
					continue
				}
				base := strings.ToLower(fields[1])
				if !isHex(base) || len(base) != 2 {
					continue
				}
				res.classBase[base] = strings.Join(fields[2:], " ")
				currentClass = base
				continue
			}

			fields := strings.Fields(trimmed)
			if len(fields) < 2 {
				continue
			}
			id := strings.ToLower(fields[0])
			if !isHex(id) || len(id) != 4 {
				continue
			}
			mode = "vendor"
			currentVendor = id
			res.vendors[id] = strings.Join(fields[1:], " ")
			continue
		}

		if indentTabs >= 2 {
			continue
		}
		if indentTabs == 1 || (indentTabs == 0 && indentSpaces > 0) {
			fields := strings.Fields(trimmed)
			if len(fields) < 2 {
				continue
			}
			id := strings.ToLower(fields[0])
			name := strings.Join(fields[1:], " ")
			switch mode {
			case "vendor":
				if !isHex(id) || len(id) != 4 {
					continue
				}
				if indentTabs == 0 && indentSpaces > 0 {
					if len(fields) >= 2 && isHex(fields[1]) && len(fields[1]) == 4 {
						continue
					}
				}
				if currentVendor == "" {
					continue
				}
				if res.devices[currentVendor] == nil {
					res.devices[currentVendor] = map[string]string{}
				}
				res.devices[currentVendor][id] = name
			case "class":
				if !isHex(id) || len(id) != 2 {
					continue
				}
				if currentClass == "" {
					continue
				}
				if res.classSub[currentClass] == nil {
					res.classSub[currentClass] = map[string]string{}
				}
				res.classSub[currentClass][id] = name
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return res, nil
}

// LoadFirst tries to load the first existing pci.ids from paths.
func LoadFirst(paths []string) (*Resolver, string, error) {
	var lastErr error
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		res, err := Load(path)
		if err == nil {
			return res, path, nil
		}
		if os.IsNotExist(err) {
			lastErr = err
			continue
		}
		return nil, path, err
	}
	if lastErr != nil {
		return nil, "", nil
	}
	return nil, "", nil
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

func leadingIndent(line string) (tabs, spaces int) {
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\t':
			tabs++
		case ' ':
			spaces++
		default:
			return tabs, spaces
		}
	}
	return tabs, spaces
}

func isHex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
