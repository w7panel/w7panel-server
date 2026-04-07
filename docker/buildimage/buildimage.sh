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
    curl -L --header "User-Agent: $USER_AGENT" -o $MODULE_NAME.tmp $DOWNLOAD_URL

    case "$DOWNLOAD_URL" in
        *.tar.gz|*.tgz)
            info "Extracting tar.gz archive..."
            tar xzf $MODULE_NAME.tmp
            ;;
        *.tar.bz2|*.tbz2)
            info "Extracting tar.bz2 archive..."
            tar xjf $MODULE_NAME.tmp
            ;;
        *.tar.xz|*.txz)
            info "Extracting tar.xz archive..."
            tar xJf $MODULE_NAME.tmp
            ;;
        *.tar)
            info "Extracting tar archive..."
            tar xf $MODULE_NAME.tmp
            ;;
        *.zip)
            info "Extracting zip archive..."
            unzip $MODULE_NAME.tmp
            ;;
        *)
            # Try to detect archive type using file command
            FILE_TYPE=$(file --brief $MODULE_NAME.tmp)
            info "Detected file type: $FILE_TYPE"
            case "$FILE_TYPE" in
                *Zip*)
                    unzip $MODULE_NAME.tmp
                    ;;
                *gzip*)
                    tar xzf $MODULE_NAME.tmp
                    ;;
                *bzip2*)
                    tar xjf $MODULE_NAME.tmp
                    ;;
                *xz*)
                    tar xJf $MODULE_NAME.tmp
                    ;;
                *tar*)
                    tar xf $MODULE_NAME.tmp
                    ;;
                *)
                    fatal "Unsupported archive format: $FILE_TYPE"
                    ;;
            esac
            ;;
    esac

    rm -f $MODULE_NAME.tmp
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

