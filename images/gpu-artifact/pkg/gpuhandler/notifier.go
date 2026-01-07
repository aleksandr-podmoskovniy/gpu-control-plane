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

package gpuhandler

// notifier pushes sync signals to the handler loop.
type notifier struct {
	ch chan struct{}
}

func newNotifier() *notifier {
	return &notifier{ch: make(chan struct{}, 1)}
}

func (n *notifier) Notify() {
	if n == nil {
		return
	}
	select {
	case n.ch <- struct{}{}:
	default:
	}
}

func (n *notifier) Channel() <-chan struct{} {
	if n == nil {
		return nil
	}
	return n.ch
}
