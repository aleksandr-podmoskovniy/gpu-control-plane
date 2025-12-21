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
	"math"
	"strconv"
	"strings"
)

func parseMemoryMiB(value string) int32 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	parts := strings.Fields(value)
	numberPart := parts[0]
	unit := ""
	if len(parts) > 1 {
		unit = strings.ToLower(parts[1])
	}

	floatVal, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		digits := extractLeadingDigits(numberPart)
		if digits == "" {
			return 0
		}
		floatVal, err = strconv.ParseFloat(digits, 64)
		if err != nil {
			return 0
		}
	}

	switch unit {
	case "gib", "gb":
		floatVal *= 1024
	case "tib", "tb":
		floatVal *= 1024 * 1024
	}

	return int32(math.Round(floatVal))
}

func parseInt32(value string) int32 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		digits := extractLeadingDigits(value)
		if digits == "" {
			return 0
		}
		number, err = strconv.Atoi(digits)
		if err != nil {
			return 0
		}
	}
	return int32(number)
}

func parseOptionalInt32(value string) *int32 {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed := parseInt32(value)
	return &parsed
}

func extractLeadingDigits(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		} else {
			break
		}
	}
	return builder.String()
}

func parseBool(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	switch value {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

