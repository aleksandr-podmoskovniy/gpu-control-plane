// Copyright 2025 Flant JSC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package settings

const (
	internalRoot = ConfigRoot + ".internal"

	InternalModuleConfigPath      = internalRoot + ".moduleConfig"
	InternalModuleValidationPath  = internalRoot + ".moduleConfigValidation"
	InternalModuleConditionsPath  = internalRoot + ".conditions"
	InternalControllerPath        = internalRoot + ".controller"
	InternalControllerCertPath    = InternalControllerPath + ".cert"
	InternalDRAControllerPath     = internalRoot + ".draController"
	InternalDRAControllerCertPath = InternalDRAControllerPath + ".cert"
	InternalMetricsPath           = internalRoot + ".metrics"
	InternalMetricsCertPath       = InternalMetricsPath + ".cert"
	InternalNodeFeatureRulePath   = internalRoot + ".nodeFeatureRule"
	InternalRootCAPath            = internalRoot + ".rootCA"
	InternalCustomCertificatePath = internalRoot + ".customCertificateData"
	InternalBootstrapStatePath    = internalRoot + ".bootstrap"
	HTTPSConfigPath               = ConfigRoot + ".https"
)
