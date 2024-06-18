{{/*
Create the container images
*/}}
{{- define "unikorn.clusterManagerControllerImage" -}}
{{- .Values.clusterManagerController.image | default (printf "%s/unikorn-cluster-manager-controller:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default .Chart.Version)) }}
{{- end }}

{{- define "unikorn.clusterControllerImage" -}}
{{- .Values.clusterController.image | default (printf "%s/unikorn-cluster-controller:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default .Chart.Version)) }}
{{- end }}

{{- define "unikorn.monitorImage" -}}
{{- .Values.monitor.image | default (printf "%s/unikorn-monitor:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default .Chart.Version)) }}
{{- end }}

{{- define "unikorn.serverImage" -}}
{{- .Values.server.image | default (printf "%s/unikorn-server:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default .Chart.Version)) }}
{{- end }}

{{/*
Create image pull secrets
*/}}
{{- define "unikorn.imagePullSecrets" -}}
{{- if .Values.imagePullSecret -}}
- name: {{ .Values.imagePullSecret }}
{{ end }}
{{- if .Values.dockerConfig -}}
- name: docker-config
{{- end }}
{{- end }}
