#!/bin/sh
set -e
set -o noglob

# --- helper functions for logs ---
info()
{
    echo '[INFO] ' "$@"
}
warn()
{
    echo '[WARN] ' "$@" >&2
}
fatal()
{
    echo '[ERROR] ' "$@" >&2
    exit 1
}
# 判断NODE_IP环境变量是否存在 存在就给/etc/hosts 添加 $NODE_IP registry.local.w7.cc
if [ -n "$NODE_IP" ]; then
    echo "$NODE_IP registry.local.w7.cc" >> /etc/hosts
    info "add $NODE_IP registry.local.w7.cc to /etc/hosts"
    info "NODE_IP is $NODE_IP"
fi

dockerauth()
{
cat > /kaniko/.docker/config.json<<EOF
$DOCKER_AUTH
EOF
}

downcode()
{
    curl -L --header "User-Agent: $USER_AGENT" -o download.tmp $DOWNLOAD_URL

    # Detect archive type using magic bytes
    HEADER=$(head -c 6 download.tmp | xxd -p 2>/dev/null || od -An -tx1 -N 6 download.tmp | tr -d ' \n')
    info "Detected file header: $HEADER"
    case "$HEADER" in
        504b0304|504b0506|504b0708)
            info "Detected ZIP archive"
            unzip download.tmp
            ;;
        1f8b*)
            info "Detected GZIP archive"
            tar xzf download.tmp
            ;;
        425a68*)
            info "Detected BZIP2 archive"
            tar xjf download.tmp
            ;;
        fd377a585a00)
            info "Detected XZ archive"
            tar xJf download.tmp
            ;;
        *)
            # Try sequential decompression methods
            if gzip -t download.tmp 2>/dev/null; then
                tar xzf download.tmp
            elif bzip2 -t download.tmp 2>/dev/null; then
                tar xjf download.tmp
            elif xz -t download.tmp 2>/dev/null; then
                tar xJf download.tmp
            elif unzip -t download.tmp 2>/dev/null; then
                unzip download.tmp
            else
                fatal "Unsupported archive format, unable to detect type"
            fi
            ;;
    esac

    rm -f download.tmp
    info "Archive extracted successfully"
}



build_kaniko()
{
/kaniko/executor --force --skip-tls-verify $INSECURE --cache=true --cache-dir=/tmp --snapshot-mode=redo \
--context=$CONTEXT \
--dockerfile=$DOCKER_FILE \
--destination=$PUSH_IMAGE
}


dockerauth
downcode
build_kaniko