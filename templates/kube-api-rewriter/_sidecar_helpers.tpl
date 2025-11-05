{{/* Copyright 2025 Flant JSC */}}

{{- define "kube_api_rewriter.kubeconfig_env" -}}
- name: KUBECONFIG
  value: /kubeconfig.local/kube-api-rewriter.kubeconfig
{{- end }}

{{- define "kube_api_rewriter.kubeconfig_volume" -}}
- name: kube-api-rewriter-kubeconfig
  configMap:
    defaultMode: 0644
    name: kube-api-rewriter-kubeconfig
{{- end }}

{{- define "kube_api_rewriter.kubeconfig_volume_mount" -}}
- name: kube-api-rewriter-kubeconfig
  mountPath: /kubeconfig.local
  readOnly: true
{{- end }}

{{- define "kube_api_rewriter.sidecar_container" -}}
{{- $ctx := index . 0 }}
- name: {{ include "kube_api_rewriter.sidecar_name" $ctx }}
  image: {{ include "helm_lib_module_image" (list $ctx "kubeApiRewriter") }}
  imagePullPolicy: IfNotPresent
  env:
    - name: LOG_LEVEL
      value: {{ include "kube_api_rewriter.log_level" $ctx | quote }}
    - name: MONITORING_BIND_ADDRESS
      value: {{ printf "127.0.0.1:%s" (include "kube_api_rewriter.monitoring_port" $ctx) | quote }}
  resources:
    requests:
      {{ include "helm_lib_module_ephemeral_storage_only_logs" $ctx | nindent 6 }}
      {{ include "kube_api_rewriter.resources" $ctx | nindent 6 }}
  securityContext:
    allowPrivilegeEscalation: false
    readOnlyRootFilesystem: true
    capabilities:
      drop:
        - ALL
    seccompProfile:
      type: RuntimeDefault
  ports:
    - containerPort: 8082
      name: https-metrics
      protocol: TCP
  livenessProbe:
    httpGet:
      path: /proxy/healthz
      port: 8082
      scheme: HTTPS
    initialDelaySeconds: 10
  readinessProbe:
    httpGet:
      path: /proxy/readyz
      port: 8082
      scheme: HTTPS
    initialDelaySeconds: 10
{{- end -}}
