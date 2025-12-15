{{/* Copyright 2025 Flant JSC */}}

{{- define "gpuControlPlane.bootstrap.componentName" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{- if eq $component "validator" -}}
{{- printf "nvidia-operator-validator" -}}
{{- else -}}
{{- printf "%s-%s" (include "gpuControlPlane.moduleName" $ctx) $component -}}
{{- end -}}
{{- end -}}

{{- define "gpuControlPlane.bootstrap.moduleLabels" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{- $role := index . 2 | default $component -}}
{{- $labels := dict "app" (include "gpuControlPlane.bootstrap.componentName" (list $ctx $component)) "component" $role -}}
{{- include "helm_lib_module_labels" (list $ctx $labels) -}}
{{- end -}}

{{- define "gpuControlPlane.bootstrap.affinity" -}}
{{- $ctx := index . 0 -}}
{{- $component := "" -}}
{{- if ge (len .) 2 -}}
  {{- $component = index . 1 -}}
{{- end -}}
{{- $managed := $ctx.Values.gpuControlPlane.managedNodes | default dict -}}
{{- $enabledByDefault := $managed.enabledByDefault | default true -}}
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
{{ include "gpuControlPlane.managedNodePresentExpression" $ctx | indent 12 }}
{{ include "gpuControlPlane.managedNodeMatchExpression" $ctx | indent 12 }}
{{- if $component }}
{{ include "gpuControlPlane.bootstrap.componentHostExpression" (list $ctx $component) | indent 12 }}
{{- end }}
        {{- if $enabledByDefault }}
        - matchExpressions:
{{ include "gpuControlPlane.managedNodePresentExpression" $ctx | indent 12 }}
{{ include "gpuControlPlane.managedNodeAbsentExpression" $ctx | indent 12 }}
{{- if $component }}
{{ include "gpuControlPlane.bootstrap.componentHostExpression" (list $ctx $component) | indent 12 }}
{{- end }}
        {{- end }}
{{- end -}}

{{- define "gpuControlPlane.bootstrap.componentEnabled" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{- $values := $ctx.Values.gpuControlPlane | default dict -}}
{{- if not (kindIs "map" $values) }}{{- $values = dict }}{{- end }}
{{- $bootstrapValues := (index $values "bootstrap") | default dict -}}
{{- if not (kindIs "map" $bootstrapValues) }}{{- $bootstrapValues = dict }}{{- end }}
{{- $componentCfg := (index $bootstrapValues $component) | default dict -}}
{{- if not (kindIs "map" $componentCfg) }}{{- $componentCfg = dict }}{{- end }}
{{- $cfgEnabled := true -}}
{{- if hasKey $componentCfg "enabled" -}}
  {{- $cfgEnabled = index $componentCfg "enabled" -}}
{{- end -}}
{{- if $cfgEnabled -}}
true
{{- else -}}
false
{{- end -}}
{{- end -}}

{{- define "gpuControlPlane.bootstrap.componentHostExpression" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{- $values := $ctx.Values.gpuControlPlane | default dict -}}
{{- if not (kindIs "map" $values) }}{{- $values = dict }}{{- end }}
{{- $internal := (index $values "internal") | default dict -}}
{{- if not (kindIs "map" $internal) }}{{- $internal = dict }}{{- end }}
{{- $bootstrap := (index $internal "bootstrap") | default dict -}}
{{- if not (kindIs "map" $bootstrap) }}{{- $bootstrap = dict }}{{- end }}
{{- $components := (index $bootstrap "components") | default dict -}}
{{- if not (kindIs "map" $components) }}{{- $components = dict }}{{- end }}
{{- $nodes := list -}}
{{- if and (hasKey $components $component) (kindIs "map" (index $components $component)) -}}
  {{- $componentData := index $components $component -}}
  {{- $stateNodes := (index $componentData "nodes") | default (list) -}}
  {{- if and (kindIs "slice" $stateNodes) (gt (len $stateNodes) 0) -}}
    {{- $nodes = $stateNodes -}}
  {{- end -}}
{{- end -}}
- key: kubernetes.io/hostname
  operator: In
  values:
{{- if and (kindIs "slice" $nodes) (gt (len $nodes) 0) }}
{{- range $nodes }}
    - {{ . | quote }}
{{- end }}
{{- else }}
    - "gpu-control-plane-no-nodes"
{{- end }}
{{- end -}}

{{- define "gpuControlPlane.bootstrap.componentHash" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{- $values := $ctx.Values.gpuControlPlane | default dict -}}
{{- if not (kindIs "map" $values) }}{{- $values = dict }}{{- end }}
{{- $internal := (index $values "internal") | default dict -}}
{{- if not (kindIs "map" $internal) }}{{- $internal = dict }}{{- end }}
{{- $bootstrap := (index $internal "bootstrap") | default dict -}}
{{- if not (kindIs "map" $bootstrap) }}{{- $bootstrap = dict }}{{- end }}
{{- $components := (index $bootstrap "components") | default dict -}}
{{- if not (kindIs "map" $components) }}{{- $components = dict }}{{- end }}
{{- $componentData := index $components $component | default dict -}}
{{- if not (kindIs "map" $componentData) }}{{- $componentData = dict }}{{- end }}
{{- $hash := "" -}}
{{- if hasKey $componentData "hash" -}}
  {{- $value := index $componentData "hash" -}}
  {{- if $value }}{{- $hash = $value -}}{{- end }}
{{- end -}}
{{- $hash -}}
{{- end -}}
