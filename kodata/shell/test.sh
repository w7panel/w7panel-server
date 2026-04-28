#!/bin/bash
kubectl get jobs -n default -o json \
  | jq -r '.items[]
    | select(
        any(.status.conditions[]?; (.type == "Complete" or .type == "Failed") and .status == "True")
      )
    | .metadata.name' \
  | xargs -r kubectl delete job -n default