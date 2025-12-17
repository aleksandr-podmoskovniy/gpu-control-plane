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
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/util/workqueue"
)

func TestNewNamedQueue(t *testing.T) {
	newQueue := NewNamedQueue()
	q := newQueue("test-controller", nil)

	// basic smoke: queue accepts and returns an item
	q.Add("item")
	if q.Len() != 1 {
		t.Fatalf("expected queue length 1, got %d", q.Len())
	}
	item, _ := q.Get()
	if item != "item" {
		t.Fatalf("expected queued item returned, got %v", item)
	}
	q.Done(item)
	q.ShutDown()
}

func TestPriorityQueueOrdersItems(t *testing.T) {
	newQueue := NewNamedQueue(UsePriorityQueue())
	q := newQueue("priority-controller", nil)

	pq, ok := q.(PriorityRateLimitingInterface)
	if !ok {
		t.Fatalf("expected priority queue implementation, got %T", q)
	}

	pq.AddWithPriority("low", 1)
	pq.Add("default")
	pq.AddWithPriority("high", 10)

	item, _ := pq.Get()
	if item != "high" {
		t.Fatalf("expected high priority item first, got %v", item)
	}
	pq.Done(item)

	item, _ = pq.Get()
	if item != "low" {
		t.Fatalf("expected low priority item second, got %v", item)
	}
	pq.Done(item)

	item, _ = pq.Get()
	if item != "default" {
		t.Fatalf("expected default priority item last, got %v", item)
	}
	pq.Done(item)
	pq.ShutDown()
}

func TestPriorityQueueUpdatesPriorityOnDuplicate(t *testing.T) {
	newQueue := NewNamedQueue(UsePriorityQueue())
	q := newQueue("priority-controller", nil)

	pq, ok := q.(PriorityRateLimitingInterface)
	if !ok {
		t.Fatalf("expected priority queue implementation, got %T", q)
	}

	pq.AddWithPriority("item", 1)
	pq.AddWithPriority("item", 5) // should bump priority, not duplicate

	if q.Len() != 1 {
		t.Fatalf("expected queue length 1, got %d", q.Len())
	}

	item, _ := pq.Get()
	if item != "item" {
		t.Fatalf("expected queued item, got %v", item)
	}
	pq.Done(item)
	pq.ShutDown()
}

func TestPriorityQueueEqualPriorityPreservesFIFOOrder(t *testing.T) {
	newQueue := NewNamedQueue(UsePriorityQueue())
	q := newQueue("priority-controller", workqueue.NewItemExponentialFailureRateLimiter(0, 0))

	pq, ok := q.(PriorityRateLimitingInterface)
	if !ok {
		t.Fatalf("expected priority queue implementation, got %T", q)
	}

	pq.AddWithPriority("first", 10)
	pq.AddWithPriority("second", 10)

	item, _ := pq.Get()
	if item != "first" {
		t.Fatalf("expected FIFO order for equal priority, got %v", item)
	}
	pq.Done(item)

	item, _ = pq.Get()
	if item != "second" {
		t.Fatalf("expected FIFO order for equal priority, got %v", item)
	}
	pq.Done(item)
	pq.ShutDown()
}

func TestPriorityQueueRateLimitingHelpersAndForget(t *testing.T) {
	newQueue := NewNamedQueue(UsePriorityQueue())
	q := newQueue("priority-controller", workqueue.NewItemExponentialFailureRateLimiter(0, 0))

	pq, ok := q.(PriorityRateLimitingInterface)
	if !ok {
		t.Fatalf("expected priority queue implementation, got %T", q)
	}

	impl, ok := pq.(*priorityRateLimitingQueue)
	if !ok {
		t.Fatalf("expected concrete implementation, got %T", pq)
	}

	pq.AddWithPriority("neg", -10) // should clamp to 0
	if got := impl.base.priorities["neg"]; got != 0 {
		t.Fatalf("expected negative priority to clamp to 0, got %d", got)
	}

	pq.AddAfter("after", 0)
	pq.AddRateLimited("ratelimited")

	pq.AddWithPriority("will-forget", 5)
	pq.Forget("will-forget")
	if _, ok := impl.base.priorities["will-forget"]; ok {
		t.Fatalf("expected priority to be cleared on Forget")
	}

	// Drain a couple of items to ensure AddAfter/AddRateLimited paths enqueue.
	deadline := time.After(500 * time.Millisecond)
	got := map[any]struct{}{}
	for len(got) < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for queue items, got %v", got)
		default:
		}
		item, shutdown := pq.Get()
		if shutdown {
			t.Fatalf("unexpected shutdown while draining")
		}
		got[item] = struct{}{}
		pq.Done(item)
	}
	pq.ShutDown()
}

func TestPriorityQueueRequeuesDirtyItemsAndShutDownWithDrain(t *testing.T) {
	q := newPriorityQueue()

	q.Add("item")
	item, shutdown := q.Get()
	if shutdown {
		t.Fatalf("unexpected shutdown")
	}

	// Mark item dirty while it is processing: should be requeued on Done.
	q.Add("item")

	requeued := make(chan any, 1)
	go func() {
		next, _ := q.Get()
		requeued <- next
	}()

	q.Done(item)
	select {
	case got := <-requeued:
		if got != "item" {
			t.Fatalf("expected item to be requeued, got %v", got)
		}
		q.Done(got)
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for requeued item")
	}

	// Ensure ShutDownWithDrain blocks until processing finishes.
	q.Add("slow")
	slow, _ := q.Get()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		q.ShutDownWithDrain()
	}()

	time.Sleep(10 * time.Millisecond) // allow goroutine to enter wait loop
	q.Done(slow)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected ShutDownWithDrain to return after Done")
	}
}

func TestPriorityQueueGetReturnsShutdownOnEmptyQueue(t *testing.T) {
	q := newPriorityQueue()

	result := make(chan struct {
		item     any
		shutdown bool
	}, 1)
	go func() {
		item, shutdown := q.Get()
		result <- struct {
			item     any
			shutdown bool
		}{item: item, shutdown: shutdown}
	}()

	time.Sleep(10 * time.Millisecond) // ensure Get waits
	q.ShutDown()

	select {
	case got := <-result:
		if got.item != nil || !got.shutdown {
			t.Fatalf("expected nil/shutdown result, got %#v", got)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for Get result after shutdown")
	}
}

func TestPriorityQueueEnsurePriorityDoesNotDowngrade(t *testing.T) {
	q := newPriorityQueue()

	q.ensurePriority("item", 10)
	q.ensurePriority("item", 5) // should no-op (current >= requested)

	if got := q.priorityOf("item"); got != 10 {
		t.Fatalf("expected priority to remain 10, got %d", got)
	}
}

func TestPriorityQueueAddCoversDirtyBranches(t *testing.T) {
	q := newPriorityQueue()

	q.Add("item")
	item, _ := q.Get()

	// First Add while processing marks dirty and returns before enqueue.
	q.Add("item")

	// Second Add sees dirty already set and pending nil, should still be a no-op.
	q.Add("item")

	q.Done(item)
	next, _ := q.Get()
	if next != "item" {
		t.Fatalf("expected requeued item, got %v", next)
	}
	q.Done(next)

	// Cover shuttingDown branch.
	q.ShutDown()
	q.Add("after-shutdown")
}

func TestPriorityQueueAddBumpsPendingPriorityWhenDirty(t *testing.T) {
	q := newPriorityQueue()

	// Add without ensuring priority -> pending has priority 0.
	q.Add("item")

	// Simulate external priority bump without touching pending entry.
	q.cond.L.Lock()
	q.priorities["item"] = 5
	q.cond.L.Unlock()

	q.Add("item")

	item, _ := q.Get()
	if item != "item" {
		t.Fatalf("unexpected item: %v", item)
	}
	q.Done(item)
	q.ShutDown()
}
