#!/usr/bin/env bash
# Run acceptance tests against a local IPAM API.
# Usage:
#   TF_ACC=1 IPAM_TOKEN=your-token ./scripts/acc-test.sh
#
# Requires: TF_ACC=1, IPAM server running (e.g. http://localhost:5173), IPAM_TOKEN set.
# Create a token in the IPAM UI: Admin → API tokens → Create token.

set -e
cd "$(dirname "$0")/.."
export TF_ACC=1
export IPAM_ENDPOINT="${IPAM_ENDPOINT:-http://localhost:5173}"
if [ -z "${IPAM_TOKEN:-}" ]; then
  echo "IPAM_TOKEN is not set. Create an API token in IPAM (Admin → API tokens) and run:"
  echo "  IPAM_TOKEN=your-token $0"
  exit 1
fi
echo "Running acceptance tests against ${IPAM_ENDPOINT} ..."
go test -v -count=1 -run TestAcc ./internal/provider/...
