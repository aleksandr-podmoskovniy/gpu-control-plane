{{/* Copyright 2025 Flant JSC */}}

{{- define "gpuControlPlane.moduleName" -}}
gpu-control-plane
{{- end -}}

{{- define "gpuControlPlane.namespace" -}}
{{- $values := .Values -}}
{{- if not (kindIs "map" $values) -}}
  {{- $values = dict -}}
{{- end -}}
{{- $module := index $values "gpuControlPlane" -}}
{{- if not (kindIs "map" $module) -}}
  {{- $module = dict -}}
{{- end -}}
{{- $bootstrap := index $module "bootstrap" -}}
{{- if not (kindIs "map" $bootstrap) -}}
  {{- $bootstrap = dict -}}
{{- end -}}
{{- default (printf "d8-%s" .Chart.Name) (index $bootstrap "namespace") -}}
{{- end -}}

{{- define "gpuControlPlane.controllerName" -}}
{{ include "gpuControlPlane.moduleName" . }}-controller
{{- end -}}

{{- define "gpuControlPlane.controllerConfigName" -}}
{{ include "gpuControlPlane.controllerName" . }}-config
{{- end -}}

{{- define "gpuControlPlane.controllerTLSSecretName" -}}
{{ include "gpuControlPlane.controllerName" . }}-tls
{{- end -}}

{{- define "gpuControlPlane.rootCASecretName" -}}
{{ include "gpuControlPlane.moduleName" . }}-ca
{{- end -}}

{{- define "gpuControlPlane.nodeFeatureRuleName" -}}
deckhouse-gpu-kernel-os
{{- end -}}

{{- define "gpuControlPlane.metricsPort" -}}8080{{- end -}}

{{- define "gpuControlPlane.selectorLabels" -}}
{{- toYaml (dict "app" (include "gpuControlPlane.controllerName" .)) -}}
{{- end -}}

{{- define "gpuControlPlane.podLabels" -}}
{{- toYaml (dict
  "app" (include "gpuControlPlane.controllerName" .)
  "module" (include "gpuControlPlane.moduleName" .)
) -}}
{{- end -}}

{{- define "gpuControlPlane.podAnnotations" -}}
{{- toYaml (dict "kubectl.kubernetes.io/default-container" "controller") -}}
{{- end -}}

{{- define "gpuControlPlane.defaultNodeSelector" -}}
{{- $ctx := index . 0 -}}
{{- $systemCount := dig "global" "discovery" "d8SpecificNodeCountByRole" "system" 0 $ctx.Values | int -}}
{{- if gt $systemCount 0 -}}
{{ include "helm_lib_node_selector" (tuple $ctx "system") }}
{{- else -}}
{{ include "helm_lib_node_selector" (tuple $ctx "worker") }}
{{- end -}}
{{- end -}}

{{- define "gpuControlPlane.isEnabled" -}}
{{- if and (hasKey .Values "gpuControlPlane") (hasKey .Values.gpuControlPlane "internal") }}
  {{- if (dig "internal" "moduleConfig" "enabled" false .Values.gpuControlPlane) -}}
true
  {{- end -}}
{{- end -}}
{{- end -}}
