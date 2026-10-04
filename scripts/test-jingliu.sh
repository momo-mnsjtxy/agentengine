#!/usr/bin/env bash
set -euo pipefail

base_url="${JINGLIU_BASE_URL:-https://ai.onyxaxis.org/v1}"
model="${JINGLIU_MODEL:-claude-opus-5}"
: "${JINGLIU_API_KEY:?请先设置 JINGLIU_API_KEY}"

echo "[1/3] endpoint: ${base_url}"
curl -fsS "${base_url}/models" \
  -H "Authorization: Bearer ${JINGLIU_API_KEY}" \
  -o /tmp/jingliu-models.json
echo "ok"

echo "[2/3] model: ${model}"
if ! grep -q "${model}" /tmp/jingliu-models.json; then
  echo "warning: model ${model} was not listed by /models; continuing with direct request" >&2
fi

echo "[3/3] completion"
curl -fsS "${base_url}/chat/completions" \
  -H "Authorization: Bearer ${JINGLIU_API_KEY}" \
  -H "Content-Type: application/json" \
  -d "$(printf '%s' '{"model":"'"${model}"'","messages":[{"role":"user","content":"Reply with exactly: jingliu-ok"}],"max_tokens":16}')"
echo
