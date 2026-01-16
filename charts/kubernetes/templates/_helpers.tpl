{{/*
Create the container images
*/}}
{{- define "unikorn.defaultTag" -}}
v{{ .Chart.Version }}
{{- end }}

{{- define "unikorn.clusterManagerControllerImage" -}}
{{- .Values.clusterManagerController.image | default (printf "%s/unikorn-cluster-manager-controller:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default (include "unikorn.defaultTag" .))) }}
{{- end }}

{{- define "unikorn.clusterControllerImage" -}}
{{- .Values.clusterController.image | default (printf "%s/unikorn-cluster-controller:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default (include "unikorn.defaultTag" .))) }}
{{- end }}

{{- define "unikorn.virtualClusterControllerImage" -}}
{{- .Values.virtualClusterController.image | default (printf "%s/unikorn-virtualcluster-controller:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default (include "unikorn.defaultTag" .))) }}
{{- end }}

{{- define "unikorn.monitorImage" -}}
{{- .Values.monitor.image | default (printf "%s/unikorn-monitor:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default (include "unikorn.defaultTag" .))) }}
{{- end }}

{{- define "unikorn.serverImage" -}}
{{- .Values.server.image | default (printf "%s/unikorn-server:%s" (include "unikorn.defaultRepositoryPath" .) (.Values.tag | default (include "unikorn.defaultTag" .))) }}
{{- end }}
