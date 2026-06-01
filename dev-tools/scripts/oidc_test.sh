#!/usr/bin/env bash
set -euo pipefail

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required" >&2
  exit 1
fi

BASE_URL="${BASE_URL:-http://127.0.0.1:8000}"
OIDC_BASE="${OIDC_BASE:-$BASE_URL/panel-api/v1/oidc}"
REGISTER_TOKEN="${REGISTER_TOKEN:-change-me}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin123456}"
REDIRECT_URI="${REDIRECT_URI:-http://127.0.0.1:3000/callback}"
CLIENT_NAME="${CLIENT_NAME:-oidc-test-client}"
COOKIE_JAR="${COOKIE_JAR:-/tmp/w7panel-oidc-cookie.txt}"

rm -f "$COOKIE_JAR"

VERIFIER="$(openssl rand -base64 64 | tr -d '=+/\n' | cut -c1-64)"
CHALLENGE="$(printf '%s' "$VERIFIER" | openssl dgst -binary -sha256 | openssl base64 -A | tr '+/' '-_' | tr -d '=')"
STATE="state-$(date +%s)"

echo "== discovery =="
curl -sS "$OIDC_BASE/.well-known/openid-configuration" | jq

echo "== dynamic client registration =="
REGISTER_RESPONSE="$(
  curl -sS -X POST "$OIDC_BASE/register" \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $REGISTER_TOKEN" \
    -d "{
      \"client_name\": \"$CLIENT_NAME\",
      \"redirect_uris\": [\"$REDIRECT_URI\"],
      \"token_endpoint_auth_method\": \"none\",
      \"grant_types\": [\"authorization_code\", \"refresh_token\"],
      \"response_types\": [\"code\"],
      \"scope\": \"openid profile offline_access\",
      \"require_pkce\": true
    }"
)"
echo "$REGISTER_RESPONSE" | jq
CLIENT_ID="$(echo "$REGISTER_RESPONSE" | jq -r '.client_id')"

echo "== get client =="
curl -sS "$OIDC_BASE/register/$CLIENT_ID" \
  -H "Authorization: Bearer $REGISTER_TOKEN" | jq

echo "== update client =="
curl -sS -X PUT "$OIDC_BASE/register/$CLIENT_ID" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $REGISTER_TOKEN" \
  -d "{
    \"client_name\": \"$CLIENT_NAME-updated\",
    \"redirect_uris\": [\"$REDIRECT_URI\"],
    \"scope\": \"openid profile offline_access\",
    \"require_pkce\": true
  }" | jq

AUTHORIZE_URL="$OIDC_BASE/authorize?response_type=code&client_id=$(printf '%s' "$CLIENT_ID" | jq -sRr @uri)&redirect_uri=$(printf '%s' "$REDIRECT_URI" | jq -sRr @uri)&scope=$(printf '%s' 'openid profile offline_access' | jq -sRr @uri)&state=$(printf '%s' "$STATE" | jq -sRr @uri)&code_challenge=$(printf '%s' "$CHALLENGE" | jq -sRr @uri)&code_challenge_method=S256"

echo "== start authorize =="
curl -sS -c "$COOKIE_JAR" "$AUTHORIZE_URL" >/dev/null

echo "== login and capture code =="
LOCATION="$(
  curl -sS -i -b "$COOKIE_JAR" -c "$COOKIE_JAR" \
    -X POST "$OIDC_BASE/authorize/login" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode 'response_type=code' \
    --data-urlencode "client_id=$CLIENT_ID" \
    --data-urlencode "redirect_uri=$REDIRECT_URI" \
    --data-urlencode 'scope=openid profile offline_access' \
    --data-urlencode "state=$STATE" \
    --data-urlencode "code_challenge=$CHALLENGE" \
    --data-urlencode 'code_challenge_method=S256' \
    --data-urlencode "username=$USERNAME" \
    --data-urlencode "password=$PASSWORD" | awk 'BEGIN{IGNORECASE=1} /^Location: /{print $2}' | tr -d '\r'
)"

if [[ -z "$LOCATION" ]]; then
  echo "failed to capture redirect location" >&2
  exit 1
fi

CODE="$(printf '%s' "$LOCATION" | sed -n 's/.*[?&]code=\([^&]*\).*/\1/p')"
if [[ -z "$CODE" ]]; then
  echo "failed to parse authorization code" >&2
  echo "$LOCATION" >&2
  exit 1
fi

echo "== exchange token =="
TOKEN_RESPONSE="$(
  curl -sS -X POST "$OIDC_BASE/token" \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    --data-urlencode 'grant_type=authorization_code' \
    --data-urlencode "client_id=$CLIENT_ID" \
    --data-urlencode "code=$CODE" \
    --data-urlencode "redirect_uri=$REDIRECT_URI" \
    --data-urlencode "code_verifier=$VERIFIER"
)"
echo "$TOKEN_RESPONSE" | jq

ACCESS_TOKEN="$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')"
REFRESH_TOKEN="$(echo "$TOKEN_RESPONSE" | jq -r '.refresh_token')"

echo "== userinfo =="
curl -sS "$OIDC_BASE/userinfo" -H "Authorization: Bearer $ACCESS_TOKEN" | jq

echo "== refresh token =="
curl -sS -X POST "$OIDC_BASE/token" \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=refresh_token' \
  --data-urlencode "client_id=$CLIENT_ID" \
  --data-urlencode "refresh_token=$REFRESH_TOKEN" | jq

echo "== delete client =="
curl -i -sS -X DELETE "$OIDC_BASE/register/$CLIENT_ID" \
  -H "Authorization: Bearer $REGISTER_TOKEN"
