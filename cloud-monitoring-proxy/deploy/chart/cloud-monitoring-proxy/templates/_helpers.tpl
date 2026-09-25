{{- define "cmproxy.fullname" -}}
{{- if contains .Chart.Name .Release.Name }}{{ .Release.Name | trunc 63 | trimSuffix "-" }}{{- else }}{{ printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}{{- end }}
{{- end }}

{{- define "cmproxy.labels" -}}
app.kubernetes.io/name: cloud-monitoring-proxy
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end }}

{{- define "cmproxy.selectorLabels" -}}
app.kubernetes.io/name: cloud-monitoring-proxy
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "cmproxy.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}{{ default (include "cmproxy.fullname" .) .Values.serviceAccount.name }}{{- else }}{{ default "default" .Values.serviceAccount.name }}{{- end }}
{{- end }}

{{- define "cmproxy.imageTag" -}}
{{ default .Chart.AppVersion .Values.image.tag }}
{{- end }}
