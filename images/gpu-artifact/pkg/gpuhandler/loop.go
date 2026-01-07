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

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/trigger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

const eventQuietPeriod = time.Second
const heartbeatPeriod = 60 * time.Second

type syncLoop struct {
	nodeName   string
	kubeConfig *rest.Config
	log        *log.Logger
}

func newSyncLoop(nodeName string, kubeConfig *rest.Config, log *log.Logger) *syncLoop {
	return &syncLoop{
		nodeName:   nodeName,
		kubeConfig: kubeConfig,
		log:        log,
	}
}

func (l *syncLoop) Run(ctx context.Context, notifier *notifier, syncFn func(context.Context) error) error {
	if l.kubeConfig == nil {
		return fmt.Errorf("kube config is required")
	}

	dyn, err := dynamic.NewForConfig(l.kubeConfig)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	sources := []trigger.Source{
		trigger.NewPhysicalGPUWatcher(dyn, l.nodeName, l.log),
	}

	errCh := make(chan error, len(sources))
	for _, source := range sources {
		source := source
		go func() {
			if err := source.Run(ctx, notifier.Notify); err != nil {
				errCh <- err
			}
		}()
	}

	timer := time.NewTimer(eventQuietPeriod)
	defer timer.Stop()
	heartbeat := time.NewTicker(heartbeatPeriod)
	defer heartbeat.Stop()

	notifyCh := notifier.Channel()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		case <-heartbeat.C:
			notifier.Notify()
		case <-notifyCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(eventQuietPeriod)
		case <-timer.C:
			if err := syncFn(ctx); err != nil {
				if l.log != nil {
					l.log.Error("sync failed", logger.SlogErr(err))
				}
				notifier.Notify()
			}
		}
	}
}
