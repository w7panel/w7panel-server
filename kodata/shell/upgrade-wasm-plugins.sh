#!/bin/sh

# 将面板历史内置 WasmPlugin 的配置迁移到制品安装的新资源。
# 新制品应设置 higress.io/wasm-plugin-name 逻辑名称和 w7.cc/group-name 制品分组标签。

set -eu

NAMESPACE="${WASM_PLUGIN_NAMESPACE:-higress-system}"
MODE="${1:-all}"
DRY_RUN="${DRY_RUN:-false}"
DELETE_LEGACY="${DELETE_LEGACY:-false}"
BACKUP_DIR="${BACKUP_DIR:-/tmp/w7panel-wasm-plugin-backup-$(date +%Y%m%d%H%M%S)}"
TARGET_WAIT_SECONDS="${TARGET_WAIT_SECONDS:-600}"
TARGET_WAIT_INTERVAL="${TARGET_WAIT_INTERVAL:-5}"

DOMAIN_LEGACY_NAME="${DOMAIN_LEGACY_NAME:-w7-white-domain}"
DOMAIN_LOGICAL_NAME="${DOMAIN_LOGICAL_NAME:-w7-white-domain}"
DOMAIN_TARGET_NAME="${DOMAIN_TARGET_NAME:-}"
DOMAIN_TARGET_GROUP="${DOMAIN_TARGET_GROUP:-}"

RATE_LIMIT_LEGACY_NAME="${RATE_LIMIT_LEGACY_NAME:-cluster-key-rate-limit}"
RATE_LIMIT_LOGICAL_NAME="${RATE_LIMIT_LOGICAL_NAME:-cluster-key-rate-limit}"
RATE_LIMIT_TARGET_NAME="${RATE_LIMIT_TARGET_NAME:-}"
RATE_LIMIT_TARGET_GROUP="${RATE_LIMIT_TARGET_GROUP:-}"

log() {
    printf '[wasm-plugin-upgrade] %s\n' "$*"
}

error() {
    printf '[wasm-plugin-upgrade] ERROR: %s\n' "$*" >&2
}

usage() {
    cat <<'EOF'
用法：upgrade-wasm-plugins.sh [all|domain|rate-limit]

前置条件：新版制品安装任务已提交。面板升级会自动完成安装并调用本脚本。

可选环境变量：
  DOMAIN_TARGET_NAME       新版域名插件的 WasmPlugin 资源名
  DOMAIN_TARGET_GROUP      新版域名插件的 w7.cc/group-name
  RATE_LIMIT_TARGET_NAME   新版限流插件的 WasmPlugin 资源名
  RATE_LIMIT_TARGET_GROUP  新版限流插件的 w7.cc/group-name
  WASM_PLUGIN_NAMESPACE    WasmPlugin 命名空间，默认 higress-system
  DELETE_LEGACY=true       迁移成功后删除旧资源，默认仅停用并移除逻辑标签
  DRY_RUN=true             只检查和展示操作，不修改资源
  BACKUP_DIR=/path         旧资源备份目录
  TARGET_WAIT_SECONDS=600  等待制品生成 WasmPlugin 的最长秒数

如果未指定 TARGET_NAME/TARGET_GROUP，脚本会按以下条件自动发现唯一目标：
  higress.io/wasm-plugin-name=<逻辑插件名>
  w7.cc/group-name 非空
EOF
}

wait_for_target() {
    legacy_name="$1"
    logical_name="$2"
    explicit_name="$3"
    target_group="$4"
    deadline="$(( $(date +%s) + TARGET_WAIT_SECONDS ))"

    while [ "$(date +%s)" -le "$deadline" ]; do
        if target_name="$(discover_target "$legacy_name" "$logical_name" "$explicit_name" "$target_group" 2>/dev/null)"; then
            printf '%s\n' "$target_name"
            return 0
        fi
        if [ "$DRY_RUN" = "true" ] || [ "$TARGET_WAIT_SECONDS" -eq 0 ]; then
            break
        fi
        log "等待制品生成 $logical_name WasmPlugin..." >&2
        sleep "$TARGET_WAIT_INTERVAL"
    done

    discover_target "$legacy_name" "$logical_name" "$explicit_name" "$target_group"
}

label_target() {
    target_name="$1"
    logical_name="$2"
    family="$3"

    if [ "$DRY_RUN" = "true" ]; then
        log "[DRY-RUN] 标记制品插件: $NAMESPACE/$target_name"
        return 0
    fi
    kubectl label wasmplugin "$target_name" -n "$NAMESPACE" \
        "higress.io/wasm-plugin-name=$logical_name" \
        "w7.cc/plugin-family=$family" --overwrite >/dev/null
}

case "$MODE" in
    all|domain|rate-limit) ;;
    -h|--help)
        usage
        exit 0
        ;;
    *)
        usage >&2
        exit 2
        ;;
esac

for command_name in kubectl jq; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
        error "缺少命令: $command_name"
        exit 1
    fi
done

umask 077
mkdir -p "$BACKUP_DIR"

resource_exists() {
    kubectl get wasmplugin "$1" -n "$NAMESPACE" >/dev/null 2>&1
}

discover_target() {
    legacy_name="$1"
    logical_name="$2"
    explicit_name="$3"
    target_group="$4"

    if [ -n "$explicit_name" ]; then
        if [ "$explicit_name" = "$legacy_name" ]; then
            error "目标资源不能与旧资源同名: $legacy_name"
            return 1
        fi
        if ! resource_exists "$explicit_name"; then
            error "指定的目标 WasmPlugin 不存在: $NAMESPACE/$explicit_name"
            return 1
        fi
        printf '%s\n' "$explicit_name"
        return 0
    fi

    if [ -n "$target_group" ]; then
        selector="w7.cc/group-name=$target_group"
    else
        selector="higress.io/wasm-plugin-name=$logical_name"
    fi

    candidates="$(kubectl get wasmplugin -n "$NAMESPACE" -l "$selector" -o json | jq -r \
        --arg legacy "$legacy_name" \
        --arg group "$target_group" '
            [.items[]
                | select(.metadata.name != $legacy)
                | select(($group != "") or ((.metadata.labels["w7.cc/group-name"] // "") != ""))
                | .metadata.name]
            | unique
            | .[]
        ')"
    count="$(printf '%s\n' "$candidates" | sed '/^$/d' | wc -l | tr -d ' ')"

    if [ "$count" -eq 0 ]; then
        error "未找到 $logical_name 的制品版 WasmPlugin，请先安装新版制品或设置 TARGET_NAME/TARGET_GROUP"
        return 1
    fi
    if [ "$count" -ne 1 ]; then
        error "$logical_name 匹配到多个制品版 WasmPlugin，请显式设置 TARGET_NAME: $candidates"
        return 1
    fi
    printf '%s\n' "$candidates"
}

patch_resource() {
    resource_name="$1"
    patch_data="$2"
    action="$3"

    if [ "$DRY_RUN" = "true" ]; then
        log "[DRY-RUN] $action: $NAMESPACE/$resource_name"
        return 0
    fi
    kubectl patch wasmplugin "$resource_name" -n "$NAMESPACE" --type=merge -p "$patch_data" >/dev/null
}

restore_legacy() {
    legacy_name="$1"
    rollback_patch="$2"
    error "迁移失败，正在恢复旧插件 $NAMESPACE/$legacy_name"
    if ! kubectl patch wasmplugin "$legacy_name" -n "$NAMESPACE" --type=merge -p "$rollback_patch" >/dev/null; then
        error "自动回滚失败，请使用备份文件手工恢复"
        return 1
    fi
    log "旧插件已恢复"
}

migrate_plugin() {
    family="$1"
    legacy_name="$2"
    logical_name="$3"
    explicit_target="$4"
    target_group="$5"

    target_name="$(wait_for_target "$legacy_name" "$logical_name" "$explicit_target" "$target_group")" || return 1
    if ! resource_exists "$legacy_name"; then
        if ! label_target "$target_name" "$logical_name" "$family"; then
            error "$family: 无法为制品插件写入逻辑标签"
            return 1
        fi
        log "$family: 未发现旧插件，制品资源 $target_name 已就绪"
        return 0
    fi
    if ! old_json="$(kubectl get wasmplugin "$legacy_name" -n "$NAMESPACE" -o json)"; then
        error "$family: 无法读取旧插件 $legacy_name"
        return 1
    fi
    if ! target_json="$(kubectl get wasmplugin "$target_name" -n "$NAMESPACE" -o json)"; then
        error "$family: 无法读取新版插件 $target_name"
        return 1
    fi

    migrated_to="$(printf '%s' "$old_json" | jq -r '.metadata.annotations["w7.cc/migrated-to"] // ""')"
    if [ "$migrated_to" = "$target_name" ]; then
        if [ "$DELETE_LEGACY" = "true" ]; then
            if [ "$DRY_RUN" = "true" ]; then
                log "[DRY-RUN] 删除已迁移的旧插件: $NAMESPACE/$legacy_name"
            elif ! kubectl delete wasmplugin "$legacy_name" -n "$NAMESPACE" >/dev/null; then
                error "$family: 无法删除已迁移的旧插件 $legacy_name"
                return 1
            fi
            log "$family: 已清理旧插件 $legacy_name"
            return 0
        fi
        log "$family: 已迁移到 ${target_name}，跳过"
        return 0
    fi

    backup_file="$BACKUP_DIR/${legacy_name}.json"
    if ! printf '%s\n' "$old_json" > "$backup_file"; then
        error "$family: 无法写入备份 $backup_file"
        return 1
    fi
    log "$family: 旧资源已备份到 $backup_file"

    if ! patches="$(jq -cn --argjson old "$old_json" --argjson target "$target_json" '
        def desired($key):
            if ($old.spec | has($key)) then $old.spec[$key] else $target.spec[$key] end;
        def plugin_annotations:
            {}
            + (if ($old.metadata.annotations["w7.cc/plugin-enabled"] // null) != null
               then {"w7.cc/plugin-enabled": $old.metadata.annotations["w7.cc/plugin-enabled"]} else {} end)
            + (if ($old.metadata.annotations["w7.cc/plugin-disabled-state"] // null) != null
               then {"w7.cc/plugin-disabled-state": $old.metadata.annotations["w7.cc/plugin-disabled-state"]} else {} end);
        {
            prepare: {
                metadata: {annotations: plugin_annotations},
                spec: ({
                    defaultConfigDisable: true,
                    matchRules: ((desired("matchRules") // []) | map(.configDisable = true))
                }
                + (if desired("defaultConfig") != null then {defaultConfig: desired("defaultConfig")} else {} end)
                )
            },
            final: {
                metadata: {annotations: plugin_annotations},
                spec: ({}
                + (if desired("defaultConfig") != null then {defaultConfig: desired("defaultConfig")} else {} end)
                + (if desired("defaultConfigDisable") != null then {defaultConfigDisable: desired("defaultConfigDisable")} else {defaultConfigDisable: null} end)
                + (if desired("matchRules") != null then {matchRules: desired("matchRules")} else {} end)
                )
            },
            disableOld: {
                spec: {
                    defaultConfigDisable: true,
                    matchRules: (($old.spec.matchRules // []) | map(.configDisable = true))
                }
            },
            rollbackOld: {
                spec: ({}
                + (if ($old.spec | has("defaultConfigDisable")) then {defaultConfigDisable: $old.spec.defaultConfigDisable} else {defaultConfigDisable: null} end)
                + (if ($old.spec | has("matchRules")) then {matchRules: $old.spec.matchRules} else {matchRules: null} end)
                )
            }
        }
    ')"; then
        error "$family: 无法生成迁移补丁"
        return 1
    fi

    if ! prepare_patch="$(printf '%s' "$patches" | jq -c '.prepare')" ||
        ! final_patch="$(printf '%s' "$patches" | jq -c '.final')" ||
        ! disable_old_patch="$(printf '%s' "$patches" | jq -c '.disableOld')" ||
        ! rollback_old_patch="$(printf '%s' "$patches" | jq -c '.rollbackOld')"; then
        error "$family: 无法解析迁移补丁"
        return 1
    fi

    patch_resource "$target_name" "$prepare_patch" "$family: 预配置并停用新版插件" || return 1
    patch_resource "$legacy_name" "$disable_old_patch" "$family: 停用旧版插件" || return 1

    if ! patch_resource "$target_name" "$final_patch" "$family: 恢复配置并启用新版插件"; then
        if [ "$DRY_RUN" != "true" ]; then
            restore_legacy "$legacy_name" "$rollback_old_patch"
        fi
        return 1
    fi

    if [ "$DRY_RUN" = "true" ]; then
        log "$family: 检查通过，实际执行时将从 $legacy_name 切换到 $target_name"
        return 0
    fi

    if ! actual_json="$(kubectl get wasmplugin "$target_name" -n "$NAMESPACE" -o json)"; then
        patch_resource "$target_name" "$prepare_patch" "$family: 读取失败，重新停用新版插件" || true
        restore_legacy "$legacy_name" "$rollback_old_patch"
        return 1
    fi
    if ! verified="$(jq -nr --argjson patch "$final_patch" --argjson actual "$actual_json" '
        [$patch.spec | to_entries[] | ($actual.spec[.key] == .value)] | all
    ')"; then
        patch_resource "$target_name" "$prepare_patch" "$family: 校验失败，重新停用新版插件" || true
        restore_legacy "$legacy_name" "$rollback_old_patch"
        return 1
    fi
    if [ "$verified" != "true" ]; then
        patch_resource "$target_name" "$prepare_patch" "$family: 校验失败，重新停用新版插件" || true
        restore_legacy "$legacy_name" "$rollback_old_patch"
        error "$family: 新插件配置校验失败"
        return 1
    fi

    if ! label_target "$target_name" "$logical_name" "$family"; then
        patch_resource "$target_name" "$prepare_patch" "$family: 标记失败，重新停用新版插件" || true
        restore_legacy "$legacy_name" "$rollback_old_patch"
        error "$family: 无法为新版插件写入逻辑标签"
        return 1
    fi
    kubectl annotate wasmplugin "$target_name" -n "$NAMESPACE" \
        "w7.cc/migrated-from=$legacy_name" --overwrite >/dev/null || true

    if [ "$DELETE_LEGACY" = "true" ]; then
        if ! kubectl delete wasmplugin "$legacy_name" -n "$NAMESPACE" >/dev/null; then
            error "$family: 新插件已启用，但旧插件删除失败，请手工删除 $legacy_name"
            return 1
        fi
        log "$family: 已删除旧插件 $legacy_name"
    else
        legacy_finalize_patch="$(jq -cn --arg target "$target_name" '{
            metadata: {
                labels: {"higress.io/wasm-plugin-name": null},
                annotations: {
                    "w7.cc/migrated-to": $target,
                    "w7.cc/migration-state": "disabled"
                }
            }
        }')"
        if ! patch_resource "$legacy_name" "$legacy_finalize_patch" "$family: 标记旧插件已迁移"; then
            patch_resource "$target_name" "$prepare_patch" "$family: 旧标签清理失败，重新停用新版插件" || true
            kubectl label wasmplugin "$target_name" -n "$NAMESPACE" \
                higress.io/wasm-plugin-name- w7.cc/plugin-family- >/dev/null 2>&1 || true
            restore_legacy "$legacy_name" "$rollback_old_patch"
            error "$family: 无法标记旧插件迁移状态"
            return 1
        fi
        log "$family: 旧插件已停用并保留，可使用 $backup_file 回滚"
    fi

    log "$family: 已成功迁移到制品资源 $target_name"
}

failed=0

if [ "$MODE" = "all" ] || [ "$MODE" = "domain" ]; then
    migrate_plugin "domain-whitelist" "$DOMAIN_LEGACY_NAME" "$DOMAIN_LOGICAL_NAME" \
        "$DOMAIN_TARGET_NAME" "$DOMAIN_TARGET_GROUP" || failed=1
fi

if [ "$MODE" = "all" ] || [ "$MODE" = "rate-limit" ]; then
    migrate_plugin "rate-limit" "$RATE_LIMIT_LEGACY_NAME" "$RATE_LIMIT_LOGICAL_NAME" \
        "$RATE_LIMIT_TARGET_NAME" "$RATE_LIMIT_TARGET_GROUP" || failed=1
fi

if [ "$failed" -ne 0 ]; then
    error "存在迁移失败的插件，备份目录: $BACKUP_DIR"
    exit 1
fi

log "迁移完成，备份目录: $BACKUP_DIR"
