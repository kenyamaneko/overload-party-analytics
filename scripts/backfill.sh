#!/bin/bash
set -euo pipefail

ENV=${1:-dev}
TABLE=${2:-all}
START_DATE=${3:-2024-01-01}

REGION="asia-northeast1"
FUNCTION_NAME="export-postgres-to-bigquery-${ENV}"

echo "=== Backfilling Historical Data ==="
echo "Environment: ${ENV}"
echo "Table: ${TABLE}"
echo "Start Date: ${START_DATE}"
echo ""

command -v gcloud >/dev/null 2>&1 || { echo "ERROR: gcloud not found in PATH" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "ERROR: curl not found in PATH" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "ERROR: jq not found in PATH (required to parse function response)" >&2; exit 1; }

# Resolve the Gen2 Cloud Function HTTPS URL once; calling via curl lets us read
# the HTTP status code, which is how the export handler signals failure.
FUNCTION_URL=$(gcloud functions describe "$FUNCTION_NAME" \
  --region "$REGION" \
  --gen2 \
  --format="value(serviceConfig.uri)" 2>/dev/null || true)

if [ -z "$FUNCTION_URL" ]; then
  echo "ERROR: failed to resolve URL for function ${FUNCTION_NAME} in ${REGION}" >&2
  exit 1
fi

# Define all tables
ALL_TABLES=("games" "game_events" "game_players" "players" "card_definitions" "deck_cards" "subscriptions" "purchases")

# Determine which tables to backfill
if [ "$TABLE" == "all" ]; then
  TABLES=("${ALL_TABLES[@]}")
else
  TABLES=("$TABLE")
fi

# Process in monthly chunks to avoid timeouts
CURRENT_DATE=$(date +%Y-%m-%d)
CHUNK_START="$START_DATE"

FAIL_COUNT=0

call_function() {
  local tbl="$1"
  local chunk_start="$2"
  local chunk_end="$3"

  local payload
  payload=$(printf '{"tables":["%s"],"mode":"full","start_date":"%s","end_date":"%s"}' \
    "$tbl" "$chunk_start" "$chunk_end")

  local id_token
  id_token=$(gcloud auth print-identity-token 2>/dev/null || true)
  if [ -z "$id_token" ]; then
    echo "ERROR: failed to mint identity token via gcloud auth print-identity-token" >&2
    return 2
  fi

  local tmpfile
  tmpfile=$(mktemp)
  local http_code
  http_code=$(curl -sS -o "$tmpfile" -w '%{http_code}' \
    -X POST "$FUNCTION_URL" \
    -H "Authorization: Bearer ${id_token}" \
    -H "Content-Type: application/json" \
    --data "$payload" || echo "000")

  local body
  body=$(cat "$tmpfile")
  rm -f "$tmpfile"

  # The handler answers 200 only when every requested table succeeded.
  if [ "$http_code" != "200" ]; then
    echo "  FAIL ${tbl} [${chunk_start} → ${chunk_end}] http=${http_code}" >&2
    echo "    response: ${body}" >&2
    return 1
  fi

  local rows
  rows=$(printf '%s' "$body" | jq -r '[.results[]?.rows_exported] | add // 0' 2>/dev/null || echo "?")
  echo "  OK   ${tbl} [${chunk_start} → ${chunk_end}] rows=${rows}"
  return 0
}

while [[ "$CHUNK_START" < "$CURRENT_DATE" ]]; do
  # Calculate chunk end (1 month later)
  CHUNK_END=$(date -d "$CHUNK_START + 1 month" +%Y-%m-%d 2>/dev/null || date -v+1m -j -f "%Y-%m-%d" "$CHUNK_START" +%Y-%m-%d)

  # Don't go past current date
  if [[ "$CHUNK_END" > "$CURRENT_DATE" ]]; then
    CHUNK_END="$CURRENT_DATE"
  fi

  for TBL in "${TABLES[@]}"; do
    echo "Backfilling ${TBL} from ${CHUNK_START} to ${CHUNK_END}..."

    if ! call_function "$TBL" "$CHUNK_START" "$CHUNK_END"; then
      FAIL_COUNT=$((FAIL_COUNT + 1))
      echo "ERROR: backfill failed for table=${TBL} range=${CHUNK_START}..${CHUNK_END}" >&2
      exit 2
    fi

    sleep 5  # Rate limiting
  done

  CHUNK_START="$CHUNK_END"
done

echo ""
if [ "$FAIL_COUNT" -eq 0 ]; then
  echo "Backfill complete!"
else
  echo "Backfill finished with ${FAIL_COUNT} failures" >&2
  exit 2
fi
