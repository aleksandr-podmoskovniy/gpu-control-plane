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

// Clone performs a deep copy of the state to guarantee isolation between store consumers.
func (s State) Clone() State {
	clone := s
	if s.Settings.DeviceApproval.Selector != nil {
		clone.Settings.DeviceApproval.Selector = s.Settings.DeviceApproval.Selector.DeepCopy()
	}
	clone.Sanitized = deepCopySanitizedMap(s.Sanitized)
	return clone
}

func deepCopySanitizedMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = deepCopyValue(value)
	}
	return dst
}

func deepCopyValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return deepCopySanitizedMap(v)
	case map[string]string:
		out := make(map[string]string, len(v))
		for key, val := range v {
			out[key] = val
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = deepCopyValue(item)
		}
		return out
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		return out
	default:
		return v
	}
}
