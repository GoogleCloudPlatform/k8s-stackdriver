# Copyright 2017 The Kubernetes Authors. All rights reserved
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

FROM golang:1.15.11
ADD . /go/src/sd-dummy-exporter
RUN go get cloud.google.com/go/compute/metadata
RUN go get golang.org/x/oauth2
RUN go get cloud.google.com/go/monitoring/apiv3
RUN CGO_ENABLED=0 GOOS=linux go install sd-dummy-exporter

FROM gcr.io/distroless/static:latest
COPY --from=0 /go/bin/sd-dummy-exporter .

