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

package physicalgpu

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/deckhouse/deckhouse/pkg/log"
)

func newScraper(ch chan<- prometheus.Metric, log *log.Logger) *scraper {
	return &scraper{ch: ch, log: log}
}

type scraper struct {
	ch  chan<- prometheus.Metric
	log *log.Logger
}

func (s *scraper) Report(m *dataMetric) {
	s.defaultUpdate(MetricPhysicalGPUInfo, 1, m)
}

func (s *scraper) defaultUpdate(name string, value float64, m *dataMetric) {
	info := physicalGPUMetrics[name]
	metric, err := prometheus.NewConstMetric(info.Desc, info.Type, value, m.labelValues()...)
	if err != nil {
		s.log.Warn(fmt.Sprintf("Error creating the new const metric for %s: %s", info.Desc, err))
		return
	}
	s.ch <- metric
}
