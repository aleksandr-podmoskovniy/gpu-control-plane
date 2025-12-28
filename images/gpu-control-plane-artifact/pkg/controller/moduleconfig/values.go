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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func (s State) Values() map[string]any {
	result := map[string]any{
		"managedNodes":   map[string]any{"labelKey": s.Settings.ManagedNodes.LabelKey, "enabledByDefault": s.Settings.ManagedNodes.EnabledByDefault},
		"deviceApproval": map[string]any{"mode": string(s.Settings.DeviceApproval.Mode)},
		"scheduling":     map[string]any{"defaultStrategy": s.Settings.Scheduling.DefaultStrategy},
		"monitoring":     map[string]any{"serviceMonitor": s.Settings.Monitoring.ServiceMonitor},
		"logLevel":       s.Settings.LogLevel,
		"inventory":      map[string]any{"resyncPeriod": s.Inventory.ResyncPeriod},
		"https":          map[string]any{"mode": string(s.HTTPS.Mode)},
		"internal":       map[string]any{"moduleConfig": map[string]any{"enabled": s.Enabled, "settings": s.Sanitized}},
	}
	if s.Settings.DeviceApproval.Selector != nil {
		result["deviceApproval"].(map[string]any)["selector"] = selectorToMap(*s.Settings.DeviceApproval.Selector)
	}
	if s.Settings.Scheduling.TopologyKey != "" {
		result["scheduling"].(map[string]any)["topologyKey"] = s.Settings.Scheduling.TopologyKey
	}
	switch s.HTTPS.Mode {
	case HTTPSModeCertManager:
		result["https"].(map[string]any)["certManager"] = map[string]any{"clusterIssuerName": s.HTTPS.CertManagerIssuer}
	case HTTPSModeCustomCertificate:
		result["https"].(map[string]any)["customCertificate"] = map[string]any{"secretName": s.HTTPS.CustomCertificateSecret}
	}
	if s.HighAvailability != nil {
		result["highAvailability"] = *s.HighAvailability
	}
	return result
}

func selectorToMap(selector metav1.LabelSelector) map[string]any {
	result := make(map[string]any)
	if len(selector.MatchLabels) > 0 {
		labels := make(map[string]string)
		for k, v := range selector.MatchLabels {
			labels[k] = v
		}
		result["matchLabels"] = labels
	}
	if len(selector.MatchExpressions) > 0 {
		expr := make([]map[string]any, 0, len(selector.MatchExpressions))
		for _, item := range selector.MatchExpressions {
			expr = append(expr, map[string]any{
				"key":      item.Key,
				"operator": string(item.Operator),
				"values":   append([]string(nil), item.Values...),
			})
		}
		result["matchExpressions"] = expr
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
