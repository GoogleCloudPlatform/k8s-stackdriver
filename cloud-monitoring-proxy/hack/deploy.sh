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

# Builds the image with Cloud Build, applies manifests, and binds the KSA
# to roles/monitoring.viewer via Workload Identity Federation (direct IAM,
# no GSA). Usage: PROJECT=<your-project> ./hack/deploy.sh
set -euo pipefail

: "${PROJECT:?PROJECT must be set to your GCP project ID}"
AR_LOCATION="${AR_LOCATION:-us-central1}"
AR_REPO="${AR_REPO:-cmproxy}"
TAG="${TAG:-e2e-$(git rev-parse --short HEAD)}"
IMAGE="$AR_LOCATION-docker.pkg.dev/$PROJECT/$AR_REPO/cloud-monitoring-proxy:$TAG"
NS=cloud-monitoring-proxy
KSA=cloud-monitoring-proxy

echo ">> building $IMAGE with Cloud Build"
gcloud builds submit --project "$PROJECT" --tag "$IMAGE" .

echo ">> applying manifests"
sed "s|IMAGE_PLACEHOLDER|$IMAGE|" deploy/manifests/proxy.yaml | kubectl apply -f -

echo ">> binding Workload Identity (direct IAM, principal uses project NUMBER)"
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT" --format='value(projectNumber)')
gcloud projects add-iam-policy-binding "$PROJECT" \
  --role roles/monitoring.viewer \
  --member "principal://iam.googleapis.com/projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/$PROJECT.svc.id.goog/subject/ns/$NS/sa/$KSA" \
  --condition=None >/dev/null
echo ">> done; waiting for rollout"
kubectl -n "$NS" rollout status deployment/cloud-monitoring-proxy --timeout=180s
