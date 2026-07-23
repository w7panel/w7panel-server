#!/bin/sh
set -e

KUBECTL="${KUBECTL:-kubectl}"
NEW_GROUP="w7panel.w7.com"
NEW_VERSION="v1alpha1"

delete_old_resource() {
    old_resource="$1"
    namespace="$2"
    name="$3"

    echo "delete ${old_resource} namespace/${namespace}/${name}: remove finalizers"
    $KUBECTL patch "${old_resource}" "${name}" -n "${namespace}" --type=merge -p '{"metadata":{"finalizers":[]}}'

    echo "delete ${old_resource} namespace/${namespace}/${name}"
    $KUBECTL delete "${old_resource}" "${name}" -n "${namespace}" --ignore-not-found
}

is_appgroup_migration() {
    old_resource="$1"
    new_resource="$2"

    [ "${old_resource}" = "appgroups.appgroup.w7.cc" ] && [ "${new_resource}" = "appgroups.w7panel.w7.com" ]
}

cleanup_migrated_appgroup() {
    old_resource="$1"
    new_resource="$2"
    namespace="$3"
    name="$4"

    # 早期版本迁移 appgroup.w7.cc -> w7panel.w7.com 后，旧 appgroup.w7.cc 对象可能因为 finalizers 没有被删除。
    # 用户在新 w7panel.w7.com AppGroup 中删除应用后，如果这个脚本再次执行，遗留的旧对象会被重新迁移回来，导致已删除应用复活。
    # 因此当新旧 AppGroup 同名对象都存在时，以新的 w7panel.w7.com 对象为准，不再对比 spec/status，直接清空旧对象 finalizers 并删除旧记录。
    echo "cleanup ${old_resource} namespace/${namespace}/${name}: ${new_resource} already exists"
    delete_old_resource "${old_resource}" "${namespace}" "${name}"
    return 0
}

cleanup_deleting_appgroup() {
    old_resource="$1"
    namespace="$2"
    name="$3"

    if [ "$($KUBECTL get "${old_resource}" "${name}" -n "${namespace}" -o json | jq -r '.metadata.deletionTimestamp // ""')" = "" ]; then
        return 1
    fi

    # 兼容已经执行过旧迁移脚本的集群：旧脚本可能已经对 appgroup.w7.cc 发起删除，
    # 但旧对象因为 finalizers 卡住，用户又在新的 w7panel.w7.com AppGroup 中删除了应用。
    # 此时新对象不存在，不能再从旧对象迁移回来；只清空旧 finalizers 并删除旧记录。
    echo "cleanup ${old_resource} namespace/${namespace}/${name}: old appgroup is already deleting"
    delete_old_resource "${old_resource}" "${namespace}" "${name}"
    return 0
}

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
            case "${name}" in
                longhorn|w7panel-longhorn|w7panel-offline)
                    echo "skip ${old_resource} namespace/${namespace}/${name}: old CRD is not migrated"
                    continue
                    ;;
            esac

            if is_appgroup_migration "${old_resource}" "${new_resource}" && cleanup_deleting_appgroup "${old_resource}" "${namespace}" "${name}"; then
                continue
            fi

            if $KUBECTL get "${new_resource}" "${name}" -n "${namespace}" >/dev/null 2>&1; then
                echo "skip ${old_resource} namespace/${namespace}/${name}: ${new_resource} already exists"
                if is_appgroup_migration "${old_resource}" "${new_resource}"; then
                    cleanup_migrated_appgroup "${old_resource}" "${new_resource}" "${namespace}" "${name}"
                    continue
                fi
                delete_old_resource "${old_resource}" "${namespace}" "${name}"
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

            delete_old_resource "${old_resource}" "${namespace}" "${name}"
        done
    done
}

migrate_resource "microapps.microapp.w7.cc" "microapps.w7panel.w7.com"
migrate_resource "appgroups.appgroup.w7.cc" "appgroups.w7panel.w7.com"
migrate_resource "gpuclasses.gpuclass.k8s.io" "gpuclasses.w7panel.w7.com"
migrate_resource "buildimages.buildimage.w7.cc" "buildimages.w7panel.w7.com"
