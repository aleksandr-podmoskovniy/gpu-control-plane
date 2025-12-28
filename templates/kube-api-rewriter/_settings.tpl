{{/* Copyright 2025 Flant JSC */}}

{{- define "kube_api_rewriter.sidecar_name" -}}proxy{{- end -}}

{{- define "kube_api_rewriter.api_port" -}}23915{{- end -}}

{{- define "kube_api_rewriter.monitoring_port" -}}9090{{- end -}}

{{- define "kube_api_rewriter.log_level" -}}
{{- include "gpuControlPlane.moduleLogLevel" . -}}
{{- end -}}

{{- define "kube_api_rewriter.resources" -}}
cpu: 50m
memory: 30Mi
{{- end -}}
