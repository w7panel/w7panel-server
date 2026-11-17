{{/*
Expand the name of the chart.
*/}}
{{- define "helm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "helm.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" $name .Release.Name  | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "helm.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "helm.labels" -}}
helm.sh/chart: {{ include "helm.chart" . }}
w7.cc/release-name: {{ .Release.Name }}
w7.cc/name: {{ include "helm.fullname" . }}
w7.cc/identifie: {{ .Chart.Name | quote }}
{{ include "helm.selectorLabels" . }}
{{ include "helm.releaseLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "helm.annotations" -}}
w7.cc/description : "微擎面板"
w7.cc/icon : "https://zpk.w7.cc/zpk/zip/icon/w7panel_offline"
w7.cc/zpk-url: "https://zpk.w7.cc/zpk/respo/info/w7panel_offline"
w7.cc/identifie: {{ .Chart.Name | quote }}
"w7.cc/deny-delete": "true"
{{- end }}

{{- define "helm.servicelb.annotations" -}}
{{- if .Values.servicelb.create -}}
w7.cc.app/ports: '{"8000": {{ .Values.servicelb.port | quote }}}'
{{- end -}}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "helm.selectorLabels" -}}
app: {{ include "helm.fullname" . }}
{{- end }}

{{- define "helm.selectorLabels-order" -}}
app: {{ include "helm.fullname" . }}-order
searchJob: {{ include "helm.fullname" . }}
w7.cc/job-source: cronjob
{{- end }}


{{- define "helm.releaseLabels" -}}
app.kubernetes.io/name: {{ include "helm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "helm.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "helm.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/* Name of the Secret shared by the panel and its Agent DaemonSet. */}}
{{- define "helm.panelAuthSecretName" -}}
{{- printf "%s-panel-auth" (include "helm.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "helm.childAgentEnv" -}}
- name: IS_CHILD
  value: "true"
- name: K3K_NAME
  value: {{ .Values.childPanel.k3kName | quote }}
- name: K3K_NAMESPACE
  value: {{ .Values.childPanel.k3kNamespace | quote }}
- name: K3K_MODE
  value: "virtual"
- name: CLUSTER_MODE
  value: "virtual"
- name: CKM_NAME
  value: {{ .Values.childPanel.ckmName | quote }}
- name: STORAGE_CLASS_NAME
  value: {{ .Values.childPanel.storageClassName | quote }}
- name: MAIN_PANEL_URL
  value: {{ .Values.childPanel.mainPanelURL | quote }}
- name: W7PANEL_AUTH_MODE
  value: {{ .Values.childPanel.authMode | quote }}
- name: OIDC_PANEL_LOGIN_ENABLED
  value: {{ .Values.childPanel.oidcLoginEnabled | quote }}
- name: OIDC_PANEL_LOGIN_DISCOVERY_URL
  value: {{ .Values.childPanel.oidcDiscoveryURL | quote }}
- name: OIDC_PANEL_LOGIN_CLIENT_ID
  value: {{ .Values.childPanel.oidcClientID | quote }}
- name: OIDC_PANEL_LOGIN_SCOPES
  value: {{ .Values.childPanel.oidcScopes | quote }}
- name: CKM_SYNC_ENDPOINT
  value: {{ .Values.childPanel.syncEndpoint | quote }}
- name: CKM_SYNC_PORT
  value: {{ .Values.childPanel.syncPort | quote }}
- name: CKM_SYNC_TOKEN
  value: {{ .Values.childPanel.syncToken | quote }}
- name: CKM_SYNC_ENABLED
  value: {{ .Values.childPanel.syncEnabled | quote }}
- name: UPGRADE_ENABLE
  value: {{ .Values.childPanel.upgradeEnabled | quote }}
- name: HELM_VERSION
  value: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
- name: IMAGE_REPO
  value: {{ .Values.image.repository | quote }}
- name: SVC_NAME
  value: {{ printf "%s-agent" (include "helm.fullname" .) | quote }}
- name: SERVICE_ACCOUNT_NAME
  value: {{ .Values.childPanel.k3kName | quote }}
- name: PANEL_SERVICE_URL
  value: {{ printf "http://%s.%s.svc:8000" (include "helm.fullname" .) .Release.Namespace | quote }}
- name: K3S_RUNTIME_DIR
  value: "/run/k3s/containerd/io.containerd.runtime.v2.task/k8s.io"
- name: NODE_IP
  valueFrom:
    fieldRef:
      fieldPath: status.hostIP
- name: POD_IP
  valueFrom:
    fieldRef:
      fieldPath: status.podIP
- name: NODE_NAME
  valueFrom:
    fieldRef:
      fieldPath: spec.nodeName
- name: TZ
  value: {{ .Values.timezone | quote }}
- name: WEBHOOK_ENABLED
  value: "true"
- name: WEBHOOK_CERT_MANAGER_ENABLED
  value: "false"
- name: APP_WATCH
  value: "true"
- name: HIGRESS_WATCH
  value: "true"
- name: K8S_WATCH
  value: "true"
- name: SITE_ENABLED
  value: "true"
- name: STATIC_DOWN_ENABLED
  value: "true"
- name: BUILDIMGAGE_ENABLED
  value: "true"
- name: K3S_REGISTRY_ENABLED
  value: "true"
{{- end }}

{{- define "helm.vmkConfig" -}}
global:
    scrape_interval: 30s
scrape_configs:
    - job-name: hami-test
      static_configs:
          - targets: ["218.23.2.55:31992"]
    - bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
      honor_timestamps: false
      job_name: kubernetes-nodes-resource
      kubernetes_sd_configs:
          - role: node
      relabel_configs:
          - action: labeldrop
            regex: __meta_kubernetes_node_label_nvidia_com.*
          - action: labeldrop
            regex: __meta_kubernetes_node_label_feature_node.*
          - action: labelmap
            regex: __meta_kubernetes_node_label_(.+)
          - replacement: kubernetes.default.svc:443
            target_label: __address__
          - regex: (.+)
            replacement: /api/v1/nodes/$1/proxy/metrics/resource
            source_labels:
                - __meta_kubernetes_node_name
            target_label: __metrics_path__
      scheme: https
      tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
          insecure_skip_verify: true
{{- end }}
