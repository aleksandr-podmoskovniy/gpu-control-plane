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

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/gpuhandler/prestart"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stdout, "%s: received SIGTERM\n", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
		os.Exit(0)
	}()

	runner := prestart.NewRunner(prestart.Options{
		DriverRoot: os.Getenv("NVIDIA_DRIVER_ROOT"),
		Out:        os.Stdout,
		Err:        os.Stderr,
	})

	if err := runner.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "prestart failed: %v\n", err)
		os.Exit(1)
	}
}
