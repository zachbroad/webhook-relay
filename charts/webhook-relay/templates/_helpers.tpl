{{/*
Expand the name of the chart.
*/}}
{{- define "webhook-relay.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "webhook-relay.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "webhook-relay.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "webhook-relay.labels" -}}
helm.sh/chart: {{ include "webhook-relay.chart" . }}
{{ include "webhook-relay.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "webhook-relay.selectorLabels" -}}
app.kubernetes.io/name: {{ include "webhook-relay.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Image tag — defaults to appVersion.
*/}}
{{- define "webhook-relay.imageTag" -}}
{{- default .Chart.AppVersion }}
{{- end }}

{{/*
Assemble the DATABASE_URL from postgres values.
*/}}
{{- define "webhook-relay.databaseUrl" -}}
{{- printf "postgres://%s:%s@%s-postgres:5432/%s?sslmode=disable" .Values.postgres.user .Values.postgres.password (include "webhook-relay.fullname" .) .Values.postgres.database }}
{{- end }}

{{/*
Assemble the REDIS_URL.
*/}}
{{- define "webhook-relay.redisUrl" -}}
{{- printf "redis://%s-redis:6379" (include "webhook-relay.fullname" .) }}
{{- end }}

{{/*
Secret name — use existing or generated.
*/}}
{{- define "webhook-relay.secretName" -}}
{{- if and (not .Values.secret.create) .Values.secret.existingSecret }}
{{- .Values.secret.existingSecret }}
{{- else }}
{{- include "webhook-relay.fullname" . }}
{{- end }}
{{- end }}
