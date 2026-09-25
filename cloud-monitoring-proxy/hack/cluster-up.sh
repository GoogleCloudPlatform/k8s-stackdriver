#!/usr/bin/env bash

# Copyright 2026 Google Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Creates the e2e GKE cluster + Artifact Registry repo.
# Usage: PROJECT=<your-project> [ZONE=us-central1-a] [CLUSTER=cmproxy-e2e] ./hack/cluster-up.sh
set -euo pipefail

: "${PROJECT:?PROJECT must be set to your GCP project ID}"
ZONE="${ZONE:-us-central1-a}"
CLUSTER="${CLUSTER:-cmproxy-e2e}"
AR_LOCATION="${AR_LOCATION:-us-central1}"
AR_REPO="${AR_REPO:-cmproxy}"

gcloud services enable container.googleapis.com artifactregistry.googleapis.com \
  cloudbuild.googleapis.com --project "$PROJECT"

gcloud artifacts repositories describe "$AR_REPO" --location "$AR_LOCATION" --project "$PROJECT" >/dev/null 2>&1 ||
  gcloud artifacts repositories create "$AR_REPO" --repository-format=docker \
    --location "$AR_LOCATION" --project "$PROJECT"

# Monitoring packages: SYSTEM (kubernetes.io/*), kube-state family, and
# cAdvisor/kubelet — everything the node/pod/kube-state presets need.
gcloud container clusters create "$CLUSTER" \
  --project "$PROJECT" --zone "$ZONE" \
  --num-nodes 2 --machine-type e2-standard-4 \
  --workload-pool="$PROJECT.svc.id.goog" \
  --monitoring=SYSTEM,POD,DEPLOYMENT,DAEMONSET,STATEFULSET,HPA,STORAGE,CADVISOR,KUBELET \
  --enable-managed-prometheus --quiet

gcloud container clusters get-credentials "$CLUSTER" --zone "$ZONE" --project "$PROJECT"
echo "cluster $CLUSTER ready"
