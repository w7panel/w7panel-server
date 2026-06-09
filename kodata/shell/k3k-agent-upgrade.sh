#!/bin/bash


if ! kubectl create -f - <<'EOF'; then
kind: ConfigMap
apiVersion: v1
metadata:
  name: longhorn-volumes-config
data:
  customs: default-volume
  default: default-volume
EOF
  echo "longhorn-volumes-config 已存在"
fi

echo "配置k3s.config crd..."
kubectl apply -f - <<'EOF' || echo "k3s.config 已更新"
kind: K3sConfig
apiVersion: w7panel.w7.com/v1alpha1
metadata:
  name: k3s.config
spec:
  data:
    k3s.mode: "4"
EOF


echo "更新higress"
helm upgrade higress https://cdn.w7.cc/w7panel/charts/higress-2.1.6.tgz \
     --namespace higress-system \
     --create-namespace \
     --version v2.1.6 \
     --set global.ingressClass=higress \
     --set higress-core.gateway.replicas=1 \
     --set higress-core.gateway.resources.limits.cpu=0 \
     --set higress-core.gateway.resources.limits.memory=0 \
     --set higress-core.gateway.resources.requests.cpu=0 \
     --set higress-core.gateway.resources.requests.memory=0 \
     --set higress-core.controller.replicas=1 \
     --set higress-core.controller.resources.requests.cpu=0 \
     --set higress-core.controller.resources.requests.memory=0 \
     --set higress-core.controller.resources.limits.cpu=0 \
     --set higress-core.controller.resources.limits.memory=0 \
     --set higress-core.pilot.replicaCount=1 \
     --set higress-core.pilot.resources.requests.cpu=0 \
     --set higress-core.pilot.resources.requests.memory=0 \
     --set higress-console.replicaCount=0 \
     --set higress-console.resources.requests.cpu=0 \
     --set higress-console.resources.requests.memory=0 --install

echo "更新cert-manager"
helm get notes cert-manager -n cert-manager || helm upgrade cert-manager $KO_DATA_PATH/charts/cert-manager-v1.19.2.tgz \
     --kubeconfig=${KUBECONFIG_PATH} \
     --namespace cert-manager \
     --create-namespace \
     --version v1.19.2 \
     --set crds.enabled=true \
     --set prometheus.enabled=false \
     --set webhook.timeoutSeconds=4 \
     --install

echo "更新cert-manager w7-letsencrypt-prod"

kubectl get ClusterIssuer/w7-letsencrypt-prod || kubectl --kubeconfig=${KUBECONFIG_PATH} apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: w7-letsencrypt-prod
spec:
  acme:
    # The ACME server URL
    server: https://acme-v02.api.letsencrypt.org/directory
    # Email address used for ACME registration
    email: 446897682@qq.com
    # Name of a secret used to store the ACME account private key
    privateKeySecretRef:
      name: w7-letsencrypt-prod
    # Enable the HTTP-01 challenge provider
    solvers:
      - http01:
          ingress:
            class: higress
EOF
           

echo "更新EnvoyFilter"
kubectl --kubeconfig=${KUBECONFIG_PATH} apply -f - <<EOF
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
    name: higress-gateway-global-route-config
    namespace: higress-system
spec:
    configPatches:
        -
            applyTo: NETWORK_FILTER
            match:
                context: GATEWAY
                listener:
                    filterChain:
                        filter:
                            name: envoy.filters.network.http_connection_manager
            patch:
                operation: MERGE
                value:
                    typed_config:
                        '@type': >-
                            type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                        skip_xff_append: true
                        xff_num_trusted_hops: 2
        -
            applyTo: ROUTE_CONFIGURATION
            match:
                context: GATEWAY
            patch:
                operation: MERGE
                value:
                    request_headers_to_add:
                        -
                            append: false
                            header:
                                key: x-real-ip
                                value: '%REQ(X-Forwarded-For)%'
                        -
                            append: false
                            header:
                                key: X-Forwarded-Proto
                                value: '%REQ(X-Forwarded-Proto)%'
EOF

# "microapp升级过需要更新crd"
kubectl apply -f $KO_DATA_PATH/crds --server-side
sh $KO_DATA_PATH/shell/migrate-crd-groups.sh

# echo "升级站点管理"
# w7panel sitemanager-upgrade --version=1.0.26 --identifie=w7_php --is-agent=true
# w7panel sitemanager-upgrade --version=1.0.26 --identifie=w7_go --is-agent=true
# w7panel sitemanager-upgrade --version=1.0.26 --identifie=w7_nodejs --is-agent=true
# w7panel sitemanager-upgrade --version=1.0.26 --identifie=w7_python --is-agent=true
# w7panel sitemanager-upgrade --version=1.0.25 --identifie=w7_sitemanager --is-agent=true
# add k3s.config

kubectl get jobs -n default -o json \
  | jq -r '.items[]
    | select(
        any(.status.conditions[]?; (.type == "Complete" or .type == "Failed") and .status == "True")
      )
    | .metadata.name' \
  | xargs -r kubectl delete job -n default
