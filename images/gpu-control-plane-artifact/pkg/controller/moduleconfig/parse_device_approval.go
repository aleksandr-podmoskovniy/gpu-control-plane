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
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func parseApproval(raw json.RawMessage) (DeviceApprovalSettings, map[string]any, error) {
	settings := DeviceApprovalSettings{Mode: DeviceApprovalModeManual}
	if len(raw) == 0 || string(raw) == "null" {
		return settings, nil, nil
	}
	var payload struct {
		Mode     string          `json:"mode"`
		Selector json.RawMessage `json:"selector"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return settings, nil, fmt.Errorf("decode deviceApproval: %w", err)
	}
	if mode := normalizeApprovalMode(payload.Mode); mode != "" {
		settings.Mode = mode
	} else if strings.TrimSpace(payload.Mode) != "" {
		return settings, nil, fmt.Errorf("unknown deviceApproval.mode %q", payload.Mode)
	}
	var selector map[string]any
	if settings.Mode == DeviceApprovalModeSelector {
		if len(payload.Selector) == 0 || string(payload.Selector) == "null" {
			return settings, nil, nil
		}
		sel, mapped, err := parseSelector(payload.Selector)
		if err != nil {
			return settings, nil, err
		}
		settings.Selector = sel
		selector = mapped
	}
	return settings, selector, nil
}

func normalizeApprovalMode(mode string) DeviceApprovalMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "":
		return ""
	case "manual":
		return DeviceApprovalModeManual
	case "automatic":
		return DeviceApprovalModeAutomatic
	case "selector":
		return DeviceApprovalModeSelector
	default:
		return ""
	}
}

func parseSelector(raw json.RawMessage) (*metav1.LabelSelector, map[string]any, error) {
	var payload struct {
		MatchLabels      map[string]string `json:"matchLabels"`
		MatchExpressions []struct {
			Key      string   `json:"key"`
			Operator string   `json:"operator"`
			Values   []string `json:"values"`
		} `json:"matchExpressions"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("decode selector: %w", err)
	}
	selector := &metav1.LabelSelector{}
	mapped := make(map[string]any)
	if len(payload.MatchLabels) > 0 {
		labels := make(map[string]string)
		selector.MatchLabels = make(map[string]string)
		for key, value := range payload.MatchLabels {
			k := strings.TrimSpace(key)
			v := strings.TrimSpace(value)
			if k == "" || v == "" {
				return nil, nil, errors.New("deviceApproval.selector.matchLabels keys and values must be non-empty")
			}
			labels[k] = v
			selector.MatchLabels[k] = v
		}
		mapped["matchLabels"] = labels
	}
	if len(payload.MatchExpressions) > 0 {
		expr := make([]metav1.LabelSelectorRequirement, 0, len(payload.MatchExpressions))
		exprMap := make([]map[string]any, 0, len(payload.MatchExpressions))
		for _, item := range payload.MatchExpressions {
			op := normalizeSelectorOperator(item.Operator)
			if op == "" {
				return nil, nil, fmt.Errorf("unsupported selector operator %q", item.Operator)
			}
			key := strings.TrimSpace(item.Key)
			if key == "" {
				return nil, nil, errors.New("deviceApproval.selector.matchExpressions[].key must be set")
			}
			values := make([]string, 0, len(item.Values))
			for _, v := range item.Values {
				if val := strings.TrimSpace(v); val != "" {
					values = append(values, val)
				}
			}
			if (op == "In" || op == "NotIn") && len(values) == 0 {
				return nil, nil, fmt.Errorf("selector operator %q requires non-empty values", op)
			}
			if (op == "Exists" || op == "DoesNotExist") && len(values) > 0 {
				return nil, nil, fmt.Errorf("selector operator %q does not accept values", op)
			}
			expr = append(expr, metav1.LabelSelectorRequirement{
				Key:      key,
				Operator: metav1.LabelSelectorOperator(op),
				Values:   values,
			})
			exprMap = append(exprMap, map[string]any{
				"key":      key,
				"operator": op,
				"values":   values,
			})
		}
		selector.MatchExpressions = expr
		mapped["matchExpressions"] = exprMap
	}
	if selector.MatchLabels == nil && len(selector.MatchExpressions) == 0 {
		return nil, nil, errors.New("deviceApproval.selector must define matchLabels or matchExpressions")
	}
	return selector, mapped, nil
}

func normalizeSelectorOperator(op string) string {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "in":
		return "In"
	case "notin":
		return "NotIn"
	case "exists":
		return "Exists"
	case "doesnotexist":
		return "DoesNotExist"
	default:
		return ""
	}
}
