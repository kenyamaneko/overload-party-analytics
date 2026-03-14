#!/bin/bash
set -e

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

# Define all tables
ALL_TABLES=("games" "game_events" "players" "matches" "card_definitions" "deck_cards" "subscriptions" "purchases")

# Determine which tables to backfill
if [ "$TABLE" == "all" ]; then
  TABLES=("${ALL_TABLES[@]}")
else
  TABLES=("$TABLE")
fi

# Process in monthly chunks to avoid timeouts
CURRENT_DATE=$(date +%Y-%m-%d)
CHUNK_START="$START_DATE"

while [[ "$CHUNK_START" < "$CURRENT_DATE" ]]; do
  # Calculate chunk end (1 month later)
  CHUNK_END=$(date -d "$CHUNK_START + 1 month" +%Y-%m-%d 2>/dev/null || date -v+1m -j -f "%Y-%m-%d" "$CHUNK_START" +%Y-%m-%d)

  # Don't go past current date
  if [[ "$CHUNK_END" > "$CURRENT_DATE" ]]; then
    CHUNK_END="$CURRENT_DATE"
  fi

  for TBL in "${TABLES[@]}"; do
    echo "Backfilling ${TBL} from ${CHUNK_START} to ${CHUNK_END}..."

    gcloud functions call "$FUNCTION_NAME" \
      --region "$REGION" \
      --data "{\"tables\": [\"$TBL\"], \"mode\": \"full\"}" \
      --quiet

    echo "  Done ${TBL}"
    sleep 5  # Rate limiting
  done

  CHUNK_START="$CHUNK_END"
done

echo ""
echo "Backfill complete!"
