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

package moduleconfig

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseLogLevel(raw json.RawMessage) (string, error) {
	level := DefaultLogLevel
	if len(raw) == 0 || string(raw) == "null" {
		return level, nil
	}
	var payload string
	if err := json.Unmarshal(raw, &payload); err != nil {
		return level, fmt.Errorf("decode logLevel: %w", err)
	}
	payload = strings.TrimSpace(payload)
	if normalized := normalizeLogLevel(payload); normalized != "" {
		return normalized, nil
	}
	if payload != "" {
		return level, fmt.Errorf("unknown logLevel %q", payload)
	}
	return level, nil
}

func normalizeLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "":
		return ""
	case "debug":
		return "Debug"
	case "info":
		return "Info"
	case "warn":
		return "Warn"
	case "error":
		return "Error"
	default:
		return ""
	}
}
