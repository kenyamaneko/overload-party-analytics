#!/usr/bin/env bash
# 入力: FUNCTION_NAME / REGION / PROJECT (env)
set -euo pipefail

: "${FUNCTION_NAME:?FUNCTION_NAME env required}"
: "${REGION:?REGION env required}"
: "${PROJECT:?PROJECT env required}"

# Cloud Functions のランタイム識別子はメジャーとマイナーまでしか持たないため、
# go.mod がパッチまで指定していても切り落とす
go_version=$(grep '^go ' export-function/go.mod | awk '{print $2}' | cut -d. -f1,2)
runtime="go${go_version//./}"

gcloud functions deploy "${FUNCTION_NAME}" \
  --gen2 \
  --region "${REGION}" \
  --project "${PROJECT}" \
  --runtime "${runtime}" \
  --entry-point ExportPostgresToBigQuery \
  --trigger-http \
  --source export-function/ \
  --quiet
