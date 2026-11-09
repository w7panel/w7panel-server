#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
mkdir -p "$test_dir/yaml/permission"
touch "$test_dir/yaml/permission/normal.yaml" "$test_dir/yaml/permission/super.yaml"
cat > "$test_dir/kubectl" <<'EOF'
#!/bin/sh
set -eu
case "$1" in
  get)
    [ "$2" = permissions.w7panel.w7.com ]
    if [ "$3" = normal ]; then
      printf '%s\n' '{"spec":{"domainWhiteList":[{"domain":"example.com"}]}}'
    fi
    ;;
  create)
    name=${4##*/}
    name=${name%.yaml}
    printf '{"metadata":{"name":"%s"},"spec":{"domainWhiteList":[],"menuRules":["new"]}}\n' "$name"
    ;;
  apply)
    if [ "$3" = - ]; then
      cat > "$TEST_DIR/normal.json"
    else
      printf '%s\n' "$3" > "$TEST_DIR/new-file"
    fi
    ;;
esac
EOF
chmod +x "$test_dir/kubectl"
PATH="$test_dir:$PATH" KO_DATA_PATH="$test_dir" TEST_DIR="$test_dir" \
  sh "$script_dir/apply-builtin-permissions.sh"
jq -e '.spec.domainWhiteList == [{"domain":"example.com"}] and .spec.menuRules == ["new"]' \
  "$test_dir/normal.json" >/dev/null
[ "$(cat "$test_dir/new-file")" = "$test_dir/yaml/permission/super.yaml" ]
