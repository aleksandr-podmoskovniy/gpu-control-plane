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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func boolPtrState(v bool) *bool { return &v }

func TestValues(t *testing.T) {
	state := State{
		Enabled: true,
		Settings: Settings{
			ManagedNodes: ManagedNodesSettings{LabelKey: "custom", EnabledByDefault: false},
			DeviceApproval: DeviceApprovalSettings{
				Mode:     DeviceApprovalModeSelector,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"gpu": "true"}},
			},
			Scheduling: SchedulingSettings{DefaultStrategy: "BinPack", TopologyKey: "zone"},
			Monitoring: MonitoringSettings{ServiceMonitor: false},
		},
		Inventory:        InventorySettings{ResyncPeriod: "45s"},
		HTTPS:            HTTPSSettings{Mode: HTTPSModeCustomCertificate, CustomCertificateSecret: "secret"},
		HighAvailability: boolPtrState(true),
		Sanitized:        map[string]any{"managedNodes": map[string]any{}},
	}

	values := state.Values()
	https := values["https"].(map[string]any)
	if https["customCertificate"].(map[string]any)["secretName"].(string) != "secret" {
		t.Fatalf("expected secret in values")
	}
	if !values["highAvailability"].(bool) {
		t.Fatalf("expected highAvailability flag")
	}
	if monitor := values["monitoring"].(map[string]any)["serviceMonitor"].(bool); monitor {
		t.Fatalf("expected serviceMonitor value propagated")
	}

	state.HTTPS = HTTPSSettings{Mode: HTTPSModeCertManager, CertManagerIssuer: "issuer"}
	values = state.Values()
	issuer := values["https"].(map[string]any)["certManager"].(map[string]any)["clusterIssuerName"].(string)
	if issuer != "issuer" {
		t.Fatalf("expected cert manager issuer in values")
	}
}

func TestCloneDeepCopy(t *testing.T) {
	state := State{
		Settings: Settings{
			ManagedNodes: ManagedNodesSettings{LabelKey: "custom", EnabledByDefault: true},
			DeviceApproval: DeviceApprovalSettings{
				Mode:     DeviceApprovalModeSelector,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"gpu": "true"}},
			},
		},
		Sanitized: map[string]any{
			"managedNodes": map[string]any{"labelKey": "custom"},
			"array":        []any{"a", "b"},
		},
	}

	clone := state.Clone()
	clone.Settings.ManagedNodes.LabelKey = "changed"
	clone.Settings.DeviceApproval.Selector.MatchLabels["gpu"] = "false"
	clone.Sanitized["managedNodes"].(map[string]any)["labelKey"] = "changed"
	clone.Sanitized["array"].([]any)[0] = "x"

	if state.Settings.ManagedNodes.LabelKey != "custom" {
		t.Fatalf("managed nodes modified in original")
	}
	if state.Settings.DeviceApproval.Selector.MatchLabels["gpu"] != "true" {
		t.Fatalf("selector modified in original")
	}
	if state.Sanitized["managedNodes"].(map[string]any)["labelKey"].(string) != "custom" {
		t.Fatalf("sanitized map modified in original")
	}
	if state.Sanitized["array"].([]any)[0].(string) != "a" {
		t.Fatalf("slice modified in original")
	}
}

func TestDeepCopySanitizedMap(t *testing.T) {
	src := map[string]any{
		"map":   map[string]any{"key": "value"},
		"slice": []any{"a", "b"},
		"num":   1,
	}
	dst := deepCopySanitizedMap(src)
	dst["map"].(map[string]any)["key"] = "changed"
	dst["slice"].([]any)[0] = "x"

	if src["map"].(map[string]any)["key"].(string) != "value" {
		t.Fatalf("expected map deep copy")
	}
	if src["slice"].([]any)[0].(string) != "a" {
		t.Fatalf("expected slice deep copy")
	}
	if deepCopySanitizedMap(nil) != nil {
		t.Fatalf("expected nil copy")
	}
}

func TestSelectorToMap(t *testing.T) {
	selector := metav1.LabelSelector{
		MatchLabels: map[string]string{"key": "value"},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "gpu", Operator: metav1.LabelSelectorOpIn, Values: []string{"true"}},
		},
	}
	out := selectorToMap(selector)
	if out["matchLabels"].(map[string]string)["key"] != "value" {
		t.Fatalf("unexpected match labels")
	}
	if len(out["matchExpressions"].([]map[string]any)) != 1 {
		t.Fatalf("unexpected match expressions")
	}
	if selectorToMap(metav1.LabelSelector{}) != nil {
		t.Fatalf("expected nil for empty selector")
	}
}

func TestDeepCopyValueVariants(t *testing.T) {
	originalMap := map[string]string{"k": "v"}
	copied := deepCopyValue(originalMap).(map[string]string)
	copied["k"] = "x"
	if originalMap["k"] != "v" {
		t.Fatalf("expected original map unchanged")
	}

	originalSlice := []string{"a", "b"}
	sliceCopy := deepCopyValue(originalSlice).([]string)
	sliceCopy[0] = "x"
	if originalSlice[0] != "a" {
		t.Fatalf("expected original slice unchanged")
	}

	if deepCopyValue(123).(int) != 123 {
		t.Fatalf("expected primitive unchanged")
	}
}
