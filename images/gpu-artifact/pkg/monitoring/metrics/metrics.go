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

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/monitoring/metrics/promutil"
)

const (
	MetricNamespace = "gpu"
)

// MetricInfo describes a prometheus metric definition.
type MetricInfo struct {
	Desc *prometheus.Desc
	Type prometheus.ValueType
}

// NewMetricInfo creates a metric descriptor with module namespace.
func NewMetricInfo(metricName, help string, t prometheus.ValueType, labels []string, constLabels prometheus.Labels) MetricInfo {
	if len(constLabels) > 0 {
		constLabels = promutil.WrapPrometheusLabels(constLabels, "", nil)
	}
	return MetricInfo{
		Desc: prometheus.NewDesc(
			prometheus.BuildFQName(MetricNamespace, "", metricName),
			help,
			labels,
			constLabels),
		Type: t,
	}
}
