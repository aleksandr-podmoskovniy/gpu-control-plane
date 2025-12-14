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
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/aleksandr-podmoskovniy/gpu-control-plane/images/kube-api-rewriter/pkg/labels"
)

type stubCounter struct {
	incCalls int
	addCalls []float64
}

func (s *stubCounter) Desc() *prometheus.Desc           { return nil }
func (s *stubCounter) Write(*dto.Metric) error          { return nil }
func (s *stubCounter) Describe(chan<- *prometheus.Desc) {}
func (s *stubCounter) Collect(chan<- prometheus.Metric) {}
func (s *stubCounter) Inc()                             { s.incCalls++ }
func (s *stubCounter) Add(v float64)                    { s.addCalls = append(s.addCalls, v) }
func (s *stubCounter) Value() (int, []float64) {
	return s.incCalls, append([]float64(nil), s.addCalls...)
}
func (s *stubCounter) AddExplicit(v float64)                      { s.addCalls = append(s.addCalls, v) }
func (s *stubCounter) GetLabelValues() map[string]string          { return map[string]string{} }
func (s *stubCounter) String() string                             { return "" }
func (s *stubCounter) SetToCurrentTime()                          {}
func (s *stubCounter) AddWithExemplar(float64, prometheus.Labels) {}
func (s *stubCounter) IncWithExemplar(prometheus.Labels)          {}
func (s *stubCounter) DescFunc(func(*prometheus.Desc))            {}
func (s *stubCounter) WriteFunc(func(*dto.Metric) error) error    { return nil }
func (s *stubCounter) DescribeFunc(func(chan<- *prometheus.Desc)) {}
func (s *stubCounter) CollectFunc(func(chan<- prometheus.Metric)) {}

type stubObserver struct {
	values []float64
}

func (s *stubObserver) Observe(v float64) {
	s.values = append(s.values, v)
}

type stubMetricsProvider struct {
	counters  map[string]*stubCounter
	observers map[string]*stubObserver
}

func newStubMetricsProvider() *stubMetricsProvider {
	return &stubMetricsProvider{
		counters:  map[string]*stubCounter{},
		observers: map[string]*stubObserver{},
	}
}

func (p *stubMetricsProvider) counter(key string) *stubCounter {
	if c, ok := p.counters[key]; ok {
		return c
	}
	c := &stubCounter{}
	p.counters[key] = c
	return c
}

func (p *stubMetricsProvider) observer(key string) *stubObserver {
	if o, ok := p.observers[key]; ok {
		return o
	}
	o := &stubObserver{}
	p.observers[key] = o
	return o
}

func (p *stubMetricsProvider) NewClientRequestsTotal(string, string, string, string, string) prometheus.Counter {
	return p.counter("clientRequests")
}
func (p *stubMetricsProvider) NewTargetResponsesTotal(string, string, string, string, string, string, string) prometheus.Counter {
	return p.counter("targetResponses")
}
func (p *stubMetricsProvider) NewTargetResponseInvalidJSONTotal(string, string, string, string, string) prometheus.Counter {
	return p.counter("targetInvalidJSON")
}
func (p *stubMetricsProvider) NewRequestsHandledTotal(string, string, string, string, string, string, string) prometheus.Counter {
	return p.counter("handled")
}
func (p *stubMetricsProvider) NewRequestsHandlingSeconds(string, string, string, string, string, string) prometheus.Observer {
	return p.observer("handlingSeconds")
}
func (p *stubMetricsProvider) NewRewritesTotal(string, string, string, string, string, string, string) prometheus.Counter {
	return p.counter("rewrites")
}
func (p *stubMetricsProvider) NewRewritesDurationSeconds(string, string, string, string, string, string) prometheus.Observer {
	return p.observer("rewriteSeconds")
}
func (p *stubMetricsProvider) NewFromClientBytesTotal(string, string, string, string, string) prometheus.Counter {
	return p.counter("fromClientBytes")
}
func (p *stubMetricsProvider) NewToTargetBytesTotal(string, string, string, string, string) prometheus.Counter {
	return p.counter("toTargetBytes")
}
func (p *stubMetricsProvider) NewFromTargetBytesTotal(string, string, string, string, string) prometheus.Counter {
	return p.counter("fromTargetBytes")
}
func (p *stubMetricsProvider) NewToClientBytesTotal(string, string, string, string, string) prometheus.Counter {
	return p.counter("toClientBytes")
}

func TestProxyMetricsCounters(t *testing.T) {
	ctx := context.Background()
	ctx = labels.ContextWithCommon(ctx, "gpu", "devices", "GET", "0", "rename", "restore")
	ctx = labels.ContextWithDecision(ctx, "rewrite")
	ctx = labels.ContextWithStatus(ctx, 201)

	provider := newStubMetricsProvider()
	pm := NewProxyMetrics(ctx, provider)

	pm.GotClientRequest()
	pm.TargetResponseSuccess("rewrite")
	pm.TargetResponseError()
	pm.TargetResponseInvalidJSON(500)
	pm.RequestHandleSuccess()
	pm.RequestHandleError()
	pm.RequestDuration(150 * time.Millisecond)
	pm.TargetResponseRewriteError()
	pm.TargetResponseRewriteSuccess()
	pm.ClientRequestRewriteError()
	pm.ClientRequestRewriteSuccess()
	pm.TargetResponseRewriteDuration(330 * time.Millisecond)
	pm.FromClientBytesAdd("pass", 10)
	pm.ToTargetBytesAdd("rewrite", 20)
	pm.FromTargetBytesAdd(30)
	pm.ToClientBytesAdd(40)

	expectCounterInc(t, provider, "clientRequests", 1)
	expectCounterInc(t, provider, "targetResponses", 2)
	expectCounterInc(t, provider, "targetInvalidJSON", 1)
	expectCounterInc(t, provider, "handled", 2)
	expectObserverCalls(t, provider, "handlingSeconds", 1)
	expectCounterInc(t, provider, "rewrites", 4)
	expectObserverCalls(t, provider, "rewriteSeconds", 1)

	expectCounterAdds(t, provider, "fromClientBytes", []float64{10})
	expectCounterAdds(t, provider, "toTargetBytes", []float64{20})
	expectCounterAdds(t, provider, "fromTargetBytes", []float64{30})
	expectCounterAdds(t, provider, "toClientBytes", []float64{40})
}

func expectCounterInc(t *testing.T, provider *stubMetricsProvider, key string, want int) {
	t.Helper()
	counter, ok := provider.counters[key]
	if !ok {
		t.Fatalf("counter %s not recorded", key)
	}
	if counter.incCalls != want {
		t.Fatalf("counter %s inc=%d want=%d", key, counter.incCalls, want)
	}
}

func expectCounterExists(t *testing.T, provider *stubMetricsProvider, key string) {
	t.Helper()
	if _, ok := provider.counters[key]; !ok {
		t.Fatalf("counter %s not recorded", key)
	}
}

func expectCounterAdds(t *testing.T, provider *stubMetricsProvider, key string, want []float64) {
	t.Helper()
	counter, ok := provider.counters[key]
	if !ok {
		t.Fatalf("counter %s not recorded", key)
	}
	if len(counter.addCalls) != len(want) {
		t.Fatalf("counter %s add len=%d want=%d", key, len(counter.addCalls), len(want))
	}
	for i, v := range want {
		if counter.addCalls[i] != v {
			t.Fatalf("counter %s add[%d]=%f want=%f", key, i, counter.addCalls[i], v)
		}
	}
}

func expectObserverCalls(t *testing.T, provider *stubMetricsProvider, key string, want int) {
	t.Helper()
	observer, ok := provider.observers[key]
	if !ok {
		t.Fatalf("observer %s not recorded", key)
	}
	if len(observer.values) != want {
		t.Fatalf("observer %s len=%d want=%d", key, len(observer.values), want)
	}
	if want > 0 && observer.values[0] <= 0 {
		t.Fatalf("observer %s must record positive durations", key)
	}
}
