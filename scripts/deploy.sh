#!/bin/bash
set -e

ENV=${1:-dev}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "=== Deploying Export Function (${ENV}) ==="

# Change to export-function directory
cd "${PROJECT_ROOT}/export-function"

# Tidy dependencies
echo "Running go mod tidy..."
go mod tidy

# Create zip file
echo "Creating function archive..."
ZIP_FILE="../function.zip"
zip -r "$ZIP_FILE" . -x "*.git*" "*.DS_Store"

# Upload to GCS
BUCKET="overload-party-${ENV}-function-source"
OBJECT="export-function-$(date +%Y%m%d-%H%M%S).zip"

echo "Uploading to gs://${BUCKET}/${OBJECT}..."
gsutil cp "$ZIP_FILE" "gs://${BUCKET}/${OBJECT}"

# Clean up
rm "$ZIP_FILE"

# Update Terraform variable
echo ""
echo "✅ Upload complete!"
echo ""
echo "Next steps:"
echo "1. cd terraform/environments/${ENV}"
echo "2. Update source_archive variable to: ${OBJECT}"
echo "3. terraform apply"
echo ""
echo "Or apply directly with:"
echo "cd terraform/environments/${ENV} && terraform apply -var=\"source_archive=${OBJECT}\""
