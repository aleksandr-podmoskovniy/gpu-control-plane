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
	"strings"
)

func parsePCIIDs(scanner *bufio.Scanner, res *Resolver) error {
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

	return scanner.Err()
}
