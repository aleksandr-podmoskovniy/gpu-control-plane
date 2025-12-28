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

{{- define "gpuControlPlane.gpuControllerName" -}}
gpu-controller
{{- end -}}

{{- define "gpuControlPlane.nodeAgentName" -}}
gpu-node-agent
{{- end -}}

{{- define "gpuControlPlane.draControllerName" -}}
gpu-dra-controller
{{- end -}}

{{- define "gpuControlPlane.handlerName" -}}
gpu-handler
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

{{- define "gpuControlPlane.metricsTLSSecretName" -}}
{{ include "gpuControlPlane.controllerName" . }}-metrics-tls
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
{{- toYaml (dict "kubectl.kubernetes.io/default-container" (include "gpuControlPlane.controllerName" .)) -}}
{{- end -}}

{{- define "gpuControlPlane.defaultNodeSelector" -}}
{{ $ctx := index . 0 }}
{{ $module := $ctx.Values.gpuControlPlane }}
{{- if and $module $module.runtime $module.runtime.controller $module.runtime.controller.nodeSelector }}
  {{- include "helm_lib_node_selector" (tuple $ctx "custom" $module.runtime.controller.nodeSelector) }}
{{- else -}}
  {{- include "helm_lib_node_selector" (tuple $ctx "master") }}
{{- end }}
{{- end -}}

{{- define "gpuControlPlane.managedNodeLabelKey" -}}
{{- $managed := .Values.gpuControlPlane.managedNodes | default dict -}}
{{- $labelKey := $managed.labelKey | default "gpu.deckhouse.io/enabled" -}}
{{- $labelKey -}}
{{- end -}}

{{- define "gpuControlPlane.managedNodeMatchExpression" -}}
{{- $managed := .Values.gpuControlPlane.managedNodes | default dict -}}
{{- $enabledByDefault := $managed.enabledByDefault | default true -}}
- key: {{ include "gpuControlPlane.managedNodeLabelKey" . }}
  {{- if $enabledByDefault }}
  operator: NotIn
  values:
    - "false"
  {{- else }}
  operator: In
  values:
    - "true"
  {{- end }}
{{- end -}}

{{- define "gpuControlPlane.managedNodePresentExpression" -}}
{{- $managed := .Values.gpuControlPlane.managedNodes | default dict -}}
{{- $presentLabel := $managed.presentLabelKey | default "" -}}
{{- if $presentLabel }}
- key: {{ $presentLabel }}
  operator: In
  values:
    - "true"
{{- end -}}
{{- end -}}

{{- define "gpuControlPlane.controlPlaneExcludedExpression" -}}
- key: node-role.kubernetes.io/control-plane
  operator: DoesNotExist
- key: node-role.kubernetes.io/master
  operator: DoesNotExist
{{- end -}}

{{- define "gpuControlPlane.managedNodeAbsentExpression" -}}
- key: {{ include "gpuControlPlane.managedNodeLabelKey" . }}
  operator: DoesNotExist
{{- end -}}

{{- define "gpuControlPlane.managedNodeAffinity" -}}
{{- $ctx := index . 0 -}}
{{- $managed := $ctx.Values.gpuControlPlane.managedNodes | default dict -}}
{{- $enabledByDefault := $managed.enabledByDefault | default true -}}
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
{{ include "gpuControlPlane.managedNodePresentExpression" $ctx | indent 10 }}
{{ include "gpuControlPlane.controlPlaneExcludedExpression" $ctx | indent 10 }}
{{ include "gpuControlPlane.managedNodeMatchExpression" $ctx | indent 10 }}
        {{- if $enabledByDefault }}
        - matchExpressions:
{{ include "gpuControlPlane.managedNodePresentExpression" $ctx | indent 10 }}
{{ include "gpuControlPlane.controlPlaneExcludedExpression" $ctx | indent 10 }}
{{ include "gpuControlPlane.managedNodeAbsentExpression" $ctx | indent 10 }}
        {{- end }}
{{- end -}}

{{- define "gpuControlPlane.nonControlPlaneAffinity" -}}
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
{{ include "gpuControlPlane.controlPlaneExcludedExpression" . | indent 10 }}
{{- end -}}

{{- define "gpuControlPlane.managedNodeTolerations" -}}
{{- $managed := .Values.gpuControlPlane.managedNodes | default dict -}}
{{- if $managed.tolerations }}
{{ include "helm_lib_tolerations" (tuple . "custom" $managed.tolerations) }}
{{- else }}
{{ include "helm_lib_tolerations" (tuple . "any-node") }}
{{- end }}
- operator: Exists
  effect: NoSchedule
- operator: Exists
  effect: NoExecute
{{- end -}}

{{- define "gpuControlPlane.isEnabled" -}}
{{- if and (hasKey .Values "gpuControlPlane") (hasKey .Values.gpuControlPlane "internal") }}
  {{- if (dig "internal" "moduleConfig" "enabled" false .Values.gpuControlPlane) -}}
true
  {{- end -}}
{{- end -}}
{{- end -}}
