//go:build linux && cgo && nvml
// +build linux,cgo,nvml

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

package capabilities

import "fmt"

type nvmlReadError struct {
	reason  error
	message string
}

func (e *nvmlReadError) Error() string {
	return e.message
}

func (e *nvmlReadError) Unwrap() error {
	return e.reason
}

func newReadError(reason error, format string, args ...interface{}) error {
	return &nvmlReadError{
		reason:  reason,
		message: fmt.Sprintf(format, args...),
	}
}
