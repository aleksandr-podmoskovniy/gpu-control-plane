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

package driver

import (
	"fmt"
	"path/filepath"

	checkpointfile "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/checkpoint/file"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/lock/fslock"
	mignvml "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/mig/nvml"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/mps"
	nvmlchecker "github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/nvml"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/timeslicing"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/adapters/vfio"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/ports"
	"github.com/aleksandr-podmoskovniy/gpu/pkg/dra/services/prepare"
)

func buildPrepareService(cfg Config, driverName, pluginPath, cdiRoot, hookPath string) (*prepare.Service, error) {
	cdiWriter, err := newCDIWriter(cdiConfig{
		DriverName:        driverName,
		DriverRoot:        cfg.DriverRoot,
		HostDriverRoot:    cfg.HostDriverRoot,
		CDIRoot:           cdiRoot,
		NvidiaCDIHookPath: hookPath,
	})
	if err != nil {
		return nil, fmt.Errorf("init CDI writer: %w", err)
	}

	locker := fslock.New(filepath.Join(pluginPath, prepareLockFile))
	checkpoints := checkpointfile.New(filepath.Join(pluginPath, prepareCheckpointFile))
	migManager := mignvml.New(mignvml.Options{DriverRoot: cfg.DriverRoot})
	vfioManager := vfio.New(vfio.Options{})
	gpuChecker := nvmlchecker.NewChecker(nvmlchecker.Options{})
	timeSlicingManager := timeslicing.New(timeslicing.Options{DriverRoot: cfg.DriverRoot})
	mpsManager := mps.New(mps.Options{DriverRoot: cfg.DriverRoot, PluginPath: pluginPath})
	prepareService, err := prepare.NewService(prepare.Options{
		CDI:               cdiWriter,
		MIG:               migManager,
		VFIO:              vfioManager,
		TimeSlicing:       timeSlicingManager,
		Mps:               mpsManager,
		Locker:            locker,
		Checkpoints:       checkpoints,
		GPUChecker:        gpuChecker,
		ResourcesNotifier: ports.NotifyFunc(cfg.ResourcesChanged),
	})
	if err != nil {
		return nil, fmt.Errorf("init prepare service: %w", err)
	}
	return prepareService, nil
}
