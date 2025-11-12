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
{{- $ctx := index . 0 -}}
{{- $component := "" -}}
{{- if ge (len .) 2 -}}
  {{- $component = index . 1 -}}
{{- end -}}
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
{{- end -}}

{{- define "gpuControlPlane.bootstrap.componentEnabled" -}}
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
{{- if not (hasKey $components $component) -}}
false
{{- else -}}
  {{- $componentData := index $components $component -}}
  {{- if not (kindIs "map" $componentData) -}}
false
  {{- else -}}
    {{- $nodes := (index $componentData "nodes") | default (list) -}}
    {{- if and (kindIs "slice" $nodes) (gt (len $nodes) 0) -}}
true
    {{- else -}}
false
    {{- end -}}
  {{- end -}}
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
{{- if and (hasKey $components $component) (kindIs "map" (index $components $component)) }}
{{- $componentData := index $components $component -}}
{{- $nodes := (index $componentData "nodes") | default (list) -}}
- key: kubernetes.io/hostname
  operator: In
  values:
{{- if and (kindIs "slice" $nodes) (gt (len $nodes) 0) }}
{{- range $nodes }}
    - {{ . | quote }}
{{- end }}
{{- else }}
    - "__bootstrap-disabled__"
{{- end }}
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
