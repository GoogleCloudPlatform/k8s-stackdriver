# Copyright 2017 Google Inc.
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

# This Dockerfile will build an image of a manual fluent-gcp scaler, allowing
# customers to override our resource request/limit defaults.

FROM gcr.io/k8s-staging-build-image/debian-base-amd64:v2.1.2

# Download latest stable version of kubectl
RUN \
    clean-install ca-certificates curl \
    && curl -v -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl \
    && chmod +x ./kubectl \
    && mv ./kubectl /usr/local/bin/kubectl \
    && apt-get purge -y --auto-remove -o APT::AutoRemove::RecommendsImportant=false curl

COPY scaler.sh /scaler.sh
