{{/* Copyright 2025 Flant JSC */}}

{{- define "gpuControlPlane.bootstrap.componentName" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{- printf "%s-%s" (include "gpuControlPlane.moduleName" $ctx) $component -}}
{{- end -}}

{{- define "gpuControlPlane.bootstrap.moduleLabels" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{- $role := index . 2 | default $component -}}
{{- $labels := dict "app" (include "gpuControlPlane.bootstrap.componentName" (list $ctx $component)) "component" $role -}}
{{- include "helm_lib_module_labels" (list $ctx $labels) -}}
{{- end -}}

{{- define "gpuControlPlane.bootstrap.affinity" -}}
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
{{ include "gpuControlPlane.managedNodePresentExpression" . | indent 12 }}
{{ include "gpuControlPlane.managedNodeMatchExpression" . | indent 12 }}
{{- end -}}
