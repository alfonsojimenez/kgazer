{{/*
Chart name, truncated to 63 chars.
*/}}
{{- define "kgazer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified app name, truncated to 63 chars.
*/}}
{{- define "kgazer.fullname" -}}
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
Common labels.
*/}}
{{- define "kgazer.labels" -}}
helm.sh/chart: {{ include "kgazer.chart" . }}
{{ include "kgazer.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "kgazer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kgazer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Chart version label.
*/}}
{{- define "kgazer.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "kgazer.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kgazer.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Database host — bundled subchart or external.
*/}}
{{- define "kgazer.databaseHost" -}}
{{- if .Values.externalDatabase.enabled }}
{{- .Values.externalDatabase.host }}
{{- else }}
{{- printf "%s-postgresql" (include "kgazer.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Database port.
*/}}
{{- define "kgazer.databasePort" -}}
{{- if .Values.externalDatabase.enabled }}
{{- .Values.externalDatabase.port | default 5432 }}
{{- else }}
{{- 5432 }}
{{- end }}
{{- end }}

{{/*
Database name.
*/}}
{{- define "kgazer.databaseName" -}}
{{- if .Values.externalDatabase.enabled }}
{{- .Values.externalDatabase.name }}
{{- else }}
{{- .Values.postgresql.auth.database }}
{{- end }}
{{- end }}

{{/*
Database user.
*/}}
{{- define "kgazer.databaseUser" -}}
{{- if .Values.externalDatabase.enabled }}
{{- .Values.externalDatabase.user }}
{{- else }}
{{- .Values.postgresql.auth.username }}
{{- end }}
{{- end }}

{{/*
Database SSL mode.
*/}}
{{- define "kgazer.databaseSSLMode" -}}
{{- if .Values.externalDatabase.enabled }}
{{- .Values.externalDatabase.sslmode | default "disable" }}
{{- else }}
{{- "disable" }}
{{- end }}
{{- end }}

{{/*
Secret name for the database password.
*/}}
{{- define "kgazer.databaseSecretName" -}}
{{- if and .Values.externalDatabase.enabled .Values.externalDatabase.existingSecret }}
{{- .Values.externalDatabase.existingSecret }}
{{- else }}
{{- include "kgazer.fullname" . }}
{{- end }}
{{- end }}

{{/*
Secret key for the database password.
*/}}
{{- define "kgazer.databaseSecretKey" -}}
{{- if and .Values.externalDatabase.enabled .Values.externalDatabase.existingSecret }}
{{- "password" }}
{{- else if .Values.externalDatabase.enabled }}
{{- "db-password" }}
{{- else }}
{{- "db-password" }}
{{- end }}
{{- end }}
