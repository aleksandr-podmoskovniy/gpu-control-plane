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

package nodeagent

import (
	"context"
	"time"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/trigger"
)

const eventQuietPeriod = time.Second

type syncLoop struct {
	log         *log.Logger
	quietPeriod time.Duration
}

func newSyncLoop(log *log.Logger) *syncLoop {
	return &syncLoop{log: log, quietPeriod: eventQuietPeriod}
}

func (l *syncLoop) Run(ctx context.Context, sources []trigger.Source, sync func(context.Context) error) error {
	notifyCh := make(chan struct{}, 1)
	notify := func() {
		select {
		case notifyCh <- struct{}{}:
		default:
		}
	}

	errCh := make(chan error, len(sources))
	for _, source := range sources {
		source := source
		go func() {
			if err := source.Run(ctx, notify); err != nil {
				errCh <- err
			}
		}()
	}

	timer := time.NewTimer(l.quietPeriod)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case <-notifyCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(l.quietPeriod)
		case <-timer.C:
			if err := sync(ctx); err != nil {
				if l.log != nil {
					l.log.Error("sync failed", logger.SlogErr(err))
				}
				notify()
			}
		}
	}
}
