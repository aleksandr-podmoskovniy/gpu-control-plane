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

package prestart

var nvidiaSMIRelDirs = []string{
	"opt/bin",
	"usr/bin",
	"usr/sbin",
	"bin",
	"sbin",
}

var nvmlLibRelDirs = []string{
	"usr/lib64",
	"usr/lib/x86_64-linux-gnu",
	"usr/lib/aarch64-linux-gnu",
	"lib64",
	"lib/x86_64-linux-gnu",
	"lib/aarch64-linux-gnu",
}
