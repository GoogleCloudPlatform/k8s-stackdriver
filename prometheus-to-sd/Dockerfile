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

FROM golang:1.9-alpine as builder
WORKDIR ${GOPATH}/src/github.com/GoogleCloudPlatform/k8s-stackdriver/prometheus-to-sd
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /monitor
RUN cp -r vendor/ /vendor/

FROM gcr.io/distroless/static
# nobody:nobody
USER 65534:65534
COPY --from=builder /monitor /

# Copies license files.
COPY --from=builder /vendor/golang.org/x/time/LICENSE /usr/share/prometheus-to-sd/vendor/golang.org/x/time/LICENSE
COPY --from=builder /vendor/golang.org/x/crypto/LICENSE /usr/share/prometheus-to-sd/vendor/golang.org/x/crypto/LICENSE
COPY --from=builder /vendor/golang.org/x/oauth2/LICENSE /usr/share/prometheus-to-sd/vendor/golang.org/x/oauth2/LICENSE
COPY --from=builder /vendor/golang.org/x/net/LICENSE /usr/share/prometheus-to-sd/vendor/golang.org/x/net/LICENSE
COPY --from=builder /vendor/golang.org/x/sys/LICENSE /usr/share/prometheus-to-sd/vendor/golang.org/x/sys/LICENSE
COPY --from=builder /vendor/cloud.google.com/go/LICENSE /usr/share/prometheus-to-sd/vendor/cloud.google.com/go/LICENSE
COPY --from=builder /vendor/k8s.io/client-go/LICENSE /usr/share/prometheus-to-sd/vendor/k8s.io/client-go/LICENSE
COPY --from=builder /vendor/k8s.io/apimachinery/LICENSE /usr/share/prometheus-to-sd/vendor/k8s.io/apimachinery/LICENSE
COPY --from=builder /vendor/k8s.io/api/LICENSE /usr/share/prometheus-to-sd/vendor/k8s.io/api/LICENSE
COPY --from=builder /vendor/github.com/modern-go/concurrent/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/modern-go/concurrent/LICENSE
COPY --from=builder /vendor/github.com/modern-go/reflect2/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/modern-go/reflect2/LICENSE
COPY --from=builder /vendor/github.com/googleapis/gnostic/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/googleapis/gnostic/LICENSE
COPY --from=builder /vendor/github.com/ghodss/yaml/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/ghodss/yaml/LICENSE
COPY --from=builder /vendor/github.com/golang/glog/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/golang/glog/LICENSE
COPY --from=builder /vendor/github.com/golang/protobuf/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/golang/protobuf/LICENSE
COPY --from=builder /vendor/github.com/matttproud/golang_protobuf_extensions/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/matttproud/golang_protobuf_extensions/LICENSE
COPY --from=builder /vendor/github.com/stretchr/testify/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/stretchr/testify/LICENSE
COPY --from=builder /vendor/github.com/pmezard/go-difflib/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/pmezard/go-difflib/LICENSE
COPY --from=builder /vendor/github.com/google/gofuzz/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/google/gofuzz/LICENSE
COPY --from=builder /vendor/github.com/google/btree/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/google/btree/LICENSE
COPY --from=builder /vendor/github.com/gogo/protobuf/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/gogo/protobuf/LICENSE
COPY --from=builder /vendor/github.com/prometheus/client_model/ruby/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/prometheus/client_model/ruby/LICENSE
COPY --from=builder /vendor/github.com/prometheus/client_model/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/prometheus/client_model/LICENSE
COPY --from=builder /vendor/github.com/prometheus/procfs/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/prometheus/procfs/LICENSE
COPY --from=builder /vendor/github.com/prometheus/common/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/prometheus/common/LICENSE
COPY --from=builder /vendor/github.com/prometheus/client_golang/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/prometheus/client_golang/LICENSE
COPY --from=builder /vendor/github.com/petar/GoLLRB/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/petar/GoLLRB/LICENSE
COPY --from=builder /vendor/github.com/beorn7/perks/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/beorn7/perks/LICENSE
COPY --from=builder /vendor/github.com/peterbourgon/diskv/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/peterbourgon/diskv/LICENSE
COPY --from=builder /vendor/github.com/davecgh/go-spew/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/davecgh/go-spew/LICENSE
COPY --from=builder /vendor/github.com/json-iterator/go/LICENSE /usr/share/prometheus-to-sd/vendor/github.com/json-iterator/go/LICENSE
COPY --from=builder /vendor/gopkg.in/yaml.v2/LICENSE /usr/share/prometheus-to-sd/vendor/gopkg.in/yaml.v2/LICENSE
COPY --from=builder /vendor/gopkg.in/inf.v0/LICENSE /usr/share/prometheus-to-sd/vendor/gopkg.in/inf.v0/LICENSE
COPY --from=builder /vendor/google.golang.org/appengine/LICENSE /usr/share/prometheus-to-sd/vendor/google.golang.org/appengine/LICENSE
COPY --from=builder /vendor/google.golang.org/api/googleapi/internal/uritemplates/LICENSE /usr/share/prometheus-to-sd/vendor/google.golang.org/api/googleapi/internal/uritemplates/LICENSE
COPY --from=builder /vendor/google.golang.org/api/LICENSE /usr/share/prometheus-to-sd/vendor/google.golang.org/api/LICENSE
