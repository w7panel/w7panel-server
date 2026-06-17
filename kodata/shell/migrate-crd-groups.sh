#!/bin/sh
set -e

KUBECTL="${KUBECTL:-kubectl}"
NEW_GROUP="w7panel.w7.com"
NEW_VERSION="v1alpha1"

migrate_resource() {
    old_resource="$1"
    new_resource="$2"
    api_version="${NEW_GROUP}/${NEW_VERSION}"

    if ! $KUBECTL get crd "${old_resource}" >/dev/null 2>&1; then
        echo "skip ${old_resource}: old CRD not found"
        return 0
    fi

    if ! $KUBECTL get crd "${new_resource}" >/dev/null 2>&1; then
        echo "skip ${old_resource}: new CRD ${new_resource} not found"
        return 1
    fi

    namespaces="$($KUBECTL get "${old_resource}" -A -o json | jq -r '.items[]?.metadata.namespace' | sort -u)"
    if [ -z "${namespaces}" ]; then
        echo "skip ${old_resource}: no objects"
        return 0
    fi

    for namespace in ${namespaces}; do
        names="$($KUBECTL get "${old_resource}" -n "${namespace}" -o json | jq -r '.items[]?.metadata.name')"
        for name in ${names}; do
            if [ "${name}" = "longhorn" ]; then
                echo "skip ${old_resource} namespace/${namespace}/${name}: longhorn old CRD is not migrated"
                continue
            fi

            if $KUBECTL get "${new_resource}" "${name}" -n "${namespace}" >/dev/null 2>&1; then
                echo "skip ${old_resource} namespace/${namespace}/${name}: ${new_resource} already exists"
                continue
            fi

            echo "migrate ${old_resource} namespace/${namespace}/${name} -> ${api_version}"
            $KUBECTL get "${old_resource}" "${name}" -n "${namespace}" -o json \
                | jq --arg apiVersion "${api_version}" '
                    .apiVersion = $apiVersion
                    | if .metadata.finalizers then
                        .metadata.finalizers = (.metadata.finalizers | map(
                            if startswith("appgroup.w7.cc/") then sub("^appgroup\\.w7\\.cc/"; "w7panel.w7.com/") else . end
                        ))
                    else . end
                    | del(
                        .metadata.uid,
                        .metadata.resourceVersion,
                        .metadata.generation,
                        .metadata.creationTimestamp,
                        .metadata.managedFields,
                        .metadata.selfLink,
                        .metadata.annotations."kubectl.kubernetes.io/last-applied-configuration"
                    )
                ' \
                | $KUBECTL apply -f -
        done
    done
}

migrate_resource "microapps.microapp.w7.cc" "microapps.w7panel.w7.com"
migrate_resource "appgroups.appgroup.w7.cc" "appgroups.w7panel.w7.com"
migrate_resource "gpuclasses.gpuclass.k8s.io" "gpuclasses.w7panel.w7.com"
migrate_resource "buildimages.buildimage.w7.cc" "buildimages.w7panel.w7.com"
