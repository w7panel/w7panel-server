#!/bin/sh
set -eu

for file in "$KO_DATA_PATH"/yaml/permission/*.yaml; do
  name=${file##*/}
  name=${name%.yaml}
  existing=$(kubectl get permissions.w7panel.w7.com "$name" --ignore-not-found -o json)
  if [ -n "$existing" ]; then
    whitelist=$(printf '%s' "$existing" | jq -c '.spec.domainWhiteList // []')
    kubectl create --dry-run=client -f "$file" -o json \
      | jq --argjson whitelist "$whitelist" '.spec.domainWhiteList = $whitelist' \
      | kubectl apply -f -
  else
    kubectl apply -f "$file"
  fi
done
