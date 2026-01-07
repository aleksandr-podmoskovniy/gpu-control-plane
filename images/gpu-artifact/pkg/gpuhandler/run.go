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
	"log/slog"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/internal/state"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

// Run starts the event-driven sync loop.
func (a *Agent) Run(ctx context.Context) error {
	if a.cfg.KubeConfig == nil {
		return fmt.Errorf("kube config is required")
	}

	notifier := newNotifier()
	bootstrap := newBootstrapService(a.cfg, a.log, a.scheme, a.store, a.reader, a.placements, a.tracker)
	result, err := bootstrap.Start(ctx, notifier.Notify)
	if err != nil {
		return err
	}

	a.draDriver = result.driver
	a.steps = result.steps
	a.stop = result.stop
	if a.stop != nil {
		defer a.stop()
	}

	loop := newSyncLoop(a.cfg.NodeName, a.cfg.KubeConfig, a.log)
	return loop.Run(ctx, notifier, a.sync)
}

func (a *Agent) sync(ctx context.Context) error {
	ctx = logger.ToContext(ctx, slog.Default())
	if a.draDriver == nil {
		return fmt.Errorf("DRA driver is not started")
	}

	st := state.New(a.cfg.NodeName)
	if _, err := a.steps.Run(ctx, st); err != nil {
		return err
	}
	a.log.Info("sync completed", "all", len(st.All()), "ready", len(st.Ready()))
	return nil
}
