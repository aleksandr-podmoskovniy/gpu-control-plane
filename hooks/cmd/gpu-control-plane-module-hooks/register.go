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
	_ "github.com/aleksandr-podmoskovniy/gpu-control-plane/hooks/pkg/hooks/bootstrap_resources"
	_ "github.com/aleksandr-podmoskovniy/gpu-control-plane/hooks/pkg/hooks/initialize_values"
	_ "github.com/aleksandr-podmoskovniy/gpu-control-plane/hooks/pkg/hooks/module_status"
	_ "github.com/aleksandr-podmoskovniy/gpu-control-plane/hooks/pkg/hooks/tls_certificates_controller"
	_ "github.com/aleksandr-podmoskovniy/gpu-control-plane/hooks/pkg/hooks/validate_module_config"
)
