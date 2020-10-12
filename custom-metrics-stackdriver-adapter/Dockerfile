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

FROM golang:1.13-alpine as builder
WORKDIR ${GOPATH}/src/github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -o /adapter
RUN ! ldd cluster-addons-bootstrap # Assert that the compiled bin is statically linked

RUN apk update \
    && apk add --no-cache govendor \
    && govendor license +vendor > /NOTICES

FROM gcr.io/distroless/static

COPY --from=builder /adapter adapter
