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

package reconciler

import (
	"container/heap"
	"sync"
	"time"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"
)

// QueueOption configures queue behavior.
type QueueOption func(*queueConfig)

type queueConfig struct {
	usePriority bool
}

// UsePriorityQueue switches the factory to priority-aware workqueue.
func UsePriorityQueue() QueueOption {
	return func(cfg *queueConfig) {
		cfg.usePriority = true
	}
}

// PriorityRateLimitingInterface exposes priority-aware enqueue helper.
type PriorityRateLimitingInterface interface {
	workqueue.RateLimitingInterface
	AddWithPriority(item interface{}, priority int)
}

// NewNamedQueue returns controller.Options.NewQueue compatible factory.
// It uses a named rate-limiting queue to expose per-controller workqueue metrics.
// When UsePriorityQueue is provided, items are dequeued by priority (higher first)
// while preserving FIFO order within the same priority.
func NewNamedQueue(opts ...QueueOption) func(controllerName string, rateLimiter ratelimiter.RateLimiter) workqueue.RateLimitingInterface {
	cfg := queueConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	return func(controllerName string, rateLimiter ratelimiter.RateLimiter) workqueue.RateLimitingInterface {
		rl := rateLimiter
		if rl == nil {
			rl = workqueue.DefaultControllerRateLimiter()
		}

		if cfg.usePriority {
			return newPriorityRateLimitingQueue(controllerName, rl)
		}

		return workqueue.NewRateLimitingQueueWithConfig(rl, workqueue.RateLimitingQueueConfig{
			Name: controllerName,
		})
	}
}

type priorityRateLimitingQueue struct {
	workqueue.RateLimitingInterface
	base *priorityQueue
}

func newPriorityRateLimitingQueue(controllerName string, rl ratelimiter.RateLimiter) PriorityRateLimitingInterface {
	base := newPriorityQueue()
	delaying := workqueue.NewDelayingQueueWithConfig(workqueue.DelayingQueueConfig{
		Name:  controllerName,
		Queue: base,
	})

	inner := workqueue.NewRateLimitingQueueWithConfig(rl, workqueue.RateLimitingQueueConfig{
		Name:          controllerName,
		DelayingQueue: delaying,
	})

	return &priorityRateLimitingQueue{
		RateLimitingInterface: inner,
		base:                  base,
	}
}

func (q *priorityRateLimitingQueue) Add(item interface{}) {
	q.base.ensurePriority(item, 0)
	q.RateLimitingInterface.Add(item)
}

func (q *priorityRateLimitingQueue) AddAfter(item interface{}, duration time.Duration) {
	q.base.ensurePriority(item, 0)
	q.RateLimitingInterface.AddAfter(item, duration)
}

func (q *priorityRateLimitingQueue) AddRateLimited(item interface{}) {
	q.base.ensurePriority(item, 0)
	q.RateLimitingInterface.AddRateLimited(item)
}

func (q *priorityRateLimitingQueue) AddWithPriority(item interface{}, priority int) {
	q.base.ensurePriority(item, priority)
	q.RateLimitingInterface.Add(item)
}

func (q *priorityRateLimitingQueue) Forget(item interface{}) {
	q.base.clearPriority(item)
	q.RateLimitingInterface.Forget(item)
}

type priorityItem struct {
	value    interface{}
	priority int
	index    int
	seq      uint64
}

type priorityHeap []*priorityItem

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	if h[i].priority == h[j].priority {
		return h[i].seq < h[j].seq
	}
	return h[i].priority > h[j].priority
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(x any) {
	item := x.(*priorityItem)
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *priorityHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	item.index = -1
	*h = old[0 : n-1]
	return item
}

type priorityQueue struct {
	cond         *sync.Cond
	queue        priorityHeap
	dirty        map[interface{}]struct{}
	processing   map[interface{}]struct{}
	pending      map[interface{}]*priorityItem
	priorities   map[interface{}]int
	shuttingDown bool
	drain        bool
	seq          uint64
}

func newPriorityQueue() *priorityQueue {
	return &priorityQueue{
		cond:       sync.NewCond(&sync.Mutex{}),
		queue:      priorityHeap{},
		dirty:      make(map[interface{}]struct{}),
		processing: make(map[interface{}]struct{}),
		pending:    make(map[interface{}]*priorityItem),
		priorities: make(map[interface{}]int),
	}
}

func (q *priorityQueue) ensurePriority(item interface{}, priority int) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if priority < 0 {
		priority = 0
	}
	current, ok := q.priorities[item]
	if ok && current >= priority {
		return
	}
	q.priorities[item] = priority

	if pending := q.pending[item]; pending != nil && priority > pending.priority {
		pending.priority = priority
		heap.Fix(&q.queue, pending.index)
	}
}

func (q *priorityQueue) clearPriority(item interface{}) {
	q.cond.L.Lock()
	delete(q.priorities, item)
	q.cond.L.Unlock()
}

func (q *priorityQueue) Add(item interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.shuttingDown {
		return
	}

	if _, exists := q.dirty[item]; exists {
		if pending := q.pending[item]; pending != nil {
			if p := q.priorityOf(item); p > pending.priority {
				pending.priority = p
				heap.Fix(&q.queue, pending.index)
			}
		}
		return
	}

	q.dirty[item] = struct{}{}
	if _, processing := q.processing[item]; processing {
		return
	}

	entry := &priorityItem{
		value:    item,
		priority: q.priorityOf(item),
		seq:      q.nextSeq(),
	}
	heap.Push(&q.queue, entry)
	q.pending[item] = entry
	q.cond.Signal()
}

func (q *priorityQueue) Len() int {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return q.queue.Len()
}

func (q *priorityQueue) Get() (item interface{}, shutdown bool) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	for q.queue.Len() == 0 && !q.shuttingDown {
		q.cond.Wait()
	}
	if q.queue.Len() == 0 {
		return nil, true
	}

	entry := heap.Pop(&q.queue).(*priorityItem)
	delete(q.pending, entry.value)
	delete(q.dirty, entry.value)
	q.processing[entry.value] = struct{}{}
	return entry.value, false
}

func (q *priorityQueue) Done(item interface{}) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	delete(q.processing, item)

	if _, dirty := q.dirty[item]; dirty {
		entry := &priorityItem{
			value:    item,
			priority: q.priorityOf(item),
			seq:      q.nextSeq(),
		}
		heap.Push(&q.queue, entry)
		q.pending[item] = entry
		q.cond.Signal()
	}

	if q.shuttingDown && len(q.processing) == 0 {
		q.cond.Broadcast()
	}
}

func (q *priorityQueue) ShutDown() {
	q.cond.L.Lock()
	q.shuttingDown = true
	q.cond.Broadcast()
	q.cond.L.Unlock()
}

func (q *priorityQueue) ShutDownWithDrain() {
	q.cond.L.Lock()
	q.shuttingDown = true
	q.drain = true
	for len(q.processing) > 0 || q.queue.Len() > 0 {
		q.cond.Wait()
	}
	q.cond.Broadcast()
	q.cond.L.Unlock()
}

func (q *priorityQueue) ShuttingDown() bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	return q.shuttingDown
}

func (q *priorityQueue) priorityOf(item interface{}) int {
	if p, ok := q.priorities[item]; ok {
		return p
	}
	return 0
}

func (q *priorityQueue) nextSeq() uint64 {
	q.seq++
	return q.seq
}
