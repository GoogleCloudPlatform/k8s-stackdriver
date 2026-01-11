#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOPATH_BIN="$(go env GOPATH)/bin"
export PATH="${GOPATH_BIN}:${PATH}"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-event-exporter-e2e}"
EVENT_COUNT="${E2E_EVENT_COUNT:-200}"
SERIES_STEP="${E2E_SERIES_STEP:-10}"
TIMEOUT="${E2E_TIMEOUT:-2m}"
KEEP_CLUSTER="${E2E_KEEP_CLUSTER:-true}"

if ! command -v kind >/dev/null 2>&1; then
  echo "kind is not installed. Please install kind first." >&2
  exit 1
fi

if ! kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  kind create cluster --name "${CLUSTER_NAME}"
fi

KUBECONFIG_FILE="$(mktemp)"
trap 'rm -f "${KUBECONFIG_FILE}"' EXIT
kind get kubeconfig --name "${CLUSTER_NAME}" > "${KUBECONFIG_FILE}"

export KUBECONFIG="${KUBECONFIG_FILE}"
export E2E_EVENT_COUNT="${EVENT_COUNT}"
export E2E_SERIES_STEP="${SERIES_STEP}"
export E2E_TIMEOUT="${TIMEOUT}"
export GOFLAGS="${GOFLAGS:-} -mod=mod"

pushd "${ROOT_DIR}" >/dev/null
go test -tags e2e ./... -run TestE2EEventsExport -count=1 -v
popd >/dev/null

if [[ "${KEEP_CLUSTER}" != "true" ]]; then
  kind delete cluster --name "${CLUSTER_NAME}"
fi
