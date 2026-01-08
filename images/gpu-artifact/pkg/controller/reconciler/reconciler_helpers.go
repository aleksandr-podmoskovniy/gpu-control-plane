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

package reconciler

import "reflect"

func handlerName[H any](h H) string {
	if named, ok := any(h).(Named); ok {
		return named.Name()
	}
	t := reflect.TypeOf(h)
	if t == nil {
		return "unknown"
	}
	if t.Kind() == reflect.Ptr {
		return t.Elem().Name()
	}
	return t.Name()
}
