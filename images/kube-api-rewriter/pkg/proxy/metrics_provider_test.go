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

package proxy

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	modulemetrics "github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/monitoring/metrics"
)

func TestRegisterMetrics(t *testing.T) {
	oldRegistry := modulemetrics.Registry
	defer func() { modulemetrics.Registry = oldRegistry }()

	modulemetrics.Registry = prometheus.NewRegistry()
	RegisterMetrics()

	metrics, err := modulemetrics.Registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatalf("expected registered metrics")
	}
}
