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

package server

import "github.com/prometheus/client_golang/prometheus"

var (
	detectRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "gfd_extender",
		Name:      "detect_requests_total",
		Help:      "Total detect requests grouped by status.",
	}, []string{"status"})

	detectWarnings = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "gfd_extender",
		Name:      "detect_warnings_total",
		Help:      "Total number of warning messages produced while collecting GPU data.",
	})

	detectDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "gfd_extender",
		Name:      "detect_duration_seconds",
		Help:      "Duration of detect requests.",
		Buckets:   prometheus.DefBuckets,
	})
)
