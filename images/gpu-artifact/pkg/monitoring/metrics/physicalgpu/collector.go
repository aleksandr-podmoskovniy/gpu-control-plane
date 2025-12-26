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
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"

	gpuv1alpha1 "github.com/aleksandr-podmoskovniy/gpu/api/v1alpha1"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const collectorName = "physicalgpu-collector"

// SetupCollector registers the PhysicalGPU metrics collector.
func SetupCollector(reader client.Reader, registerer prometheus.Registerer, log *log.Logger) {
	c := NewCollector(reader, log)
	c.Register(registerer)
}

// Collector exposes PhysicalGPU metrics.
type Collector struct {
	reader client.Reader
	log    *log.Logger
}

// NewCollector constructs a PhysicalGPU metrics collector.
func NewCollector(reader client.Reader, log *log.Logger) *Collector {
	return &Collector{
		reader: reader,
		log:    log.With(logger.SlogCollector(collectorName)),
	}
}

// Register registers the collector in the registry.
func (c *Collector) Register(reg prometheus.Registerer) {
	reg.MustRegister(c)
}

// Describe describes all metrics.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range physicalGPUMetrics {
		ch <- m.Desc
	}
}

// Collect collects all metrics.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	s := newScraper(ch, c.log)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	list := &gpuv1alpha1.PhysicalGPUList{}
	if err := c.reader.List(ctx, list); err != nil {
		c.log.Error("Failed to list PhysicalGPUs", logger.SlogErr(err))
		return
	}

	for i := range list.Items {
		metric := newDataMetric(&list.Items[i])
		s.Report(metric)
	}
}
