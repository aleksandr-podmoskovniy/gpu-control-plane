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

package webhook

import (
	"github.com/deckhouse/deckhouse/pkg/log"

	"github.com/aleksandr-podmoskovniy/gpu/pkg/logger"
)

func ensureLog(l *log.Logger) *log.Logger {
	if l == nil {
		l = log.NewNop()
	}
	return l.With(logger.SlogHandler("dra-claim-webhook"))
}

func newHandler(log *log.Logger, driverName string) *ParametersHandler {
	return NewParametersHandler(ensureLog(log), driverName)
}
