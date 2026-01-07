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
	"log/slog"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

// Run starts the event-driven sync loop.
func (a *Agent) Run(ctx context.Context) error {
	bootstrap := newBootstrapService(a.cfg, a.log, a.scheme, a.store, a.pci, a.hostInfo)
	if err := bootstrap.validate(); err != nil {
		return err
	}
	result, err := bootstrap.Start()
	if err != nil {
		return err
	}
	if result.stop != nil {
		defer result.stop()
	}
	a.steps = result.steps

	sources, err := buildSources(a.cfg, a.log)
	if err != nil {
		return err
	}

	loop := newSyncLoop(a.log)
	return loop.Run(ctx, sources, a.sync)
}

func (a *Agent) sync(ctx context.Context) error {
	ctx = logger.ToContext(ctx, slog.Default())
	st := state.New(a.cfg.NodeName)
	if _, err := a.steps.Run(ctx, st); err != nil {
		return err
	}
	a.log.Info("sync completed", "devices", len(st.Devices()))
	return nil
}
