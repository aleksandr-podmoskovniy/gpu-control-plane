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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/common/steptaker"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/eventrecord"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/handler"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/handler/apply"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/handler/cleanup"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/handler/discover"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/service"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/nodeagent/internal/state"
)

const nodeAgentComponent = "gpu-node-agent"

type bootstrapService struct {
	cfg      Config
	log      *log.Logger
	scheme   *runtime.Scheme
	store    service.Store
	pci      service.PCIProvider
	hostInfo service.HostInfoProvider
}

type bootstrapResult struct {
	steps steptaker.StepTakers[state.State]
	stop  func()
}

func newBootstrapService(cfg Config, log *log.Logger, scheme *runtime.Scheme, store service.Store, pci service.PCIProvider, hostInfo service.HostInfoProvider) *bootstrapService {
	return &bootstrapService{
		cfg:      cfg,
		log:      log,
		scheme:   scheme,
		store:    store,
		pci:      pci,
		hostInfo: hostInfo,
	}
}

func (b *bootstrapService) Start() (*bootstrapResult, error) {
	recorder, stopRecorder := b.startEventRecorder()
	steps := handler.NewSteps(
		b.log,
		discover.NewDiscoverHandler(b.pci, b.hostInfo),
		apply.NewApplyHandler(b.store, recorder),
		cleanup.NewCleanupHandler(b.store, recorder),
	)

	stop := func() {
		if stopRecorder != nil {
			stopRecorder()
		}
	}

	return &bootstrapResult{steps: steps, stop: stop}, nil
}

func (b *bootstrapService) startEventRecorder() (eventrecord.EventRecorderLogger, func()) {
	if b.scheme == nil {
		return nil, nil
	}

	kubeClient, err := kubernetes.NewForConfig(b.cfg.KubeConfig)
	if err != nil {
		if b.log != nil {
			b.log.Error("unable to create kube clientset for events", logger.SlogErr(err))
		}
		return nil, nil
	}

	recorder, stop := newEventRecorder(kubeClient, b.scheme, nodeAgentComponent)
	if recorder == nil {
		return nil, stop
	}
	if b.log == nil {
		return recorder, stop
	}
	return recorder.WithLogging(b.log.With(logger.SlogController(nodeAgentComponent))), stop
}

func (b *bootstrapService) validate() error {
	if b.cfg.KubeConfig == nil {
		return fmt.Errorf("kube config is required")
	}
	return nil
}
