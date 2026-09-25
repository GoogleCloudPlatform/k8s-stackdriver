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

IMAGE_NAME = cloud-monitoring-proxy
PREFIX ?= staging-k8s.gcr.io
TAG ?= v0.1.0
IMAGE = $(PREFIX)/$(IMAGE_NAME)

.PHONY: all build test lint run container push clean e2e

all: build

build:
	CGO_ENABLED=0 go build -mod=readonly -o bin/cloud-monitoring-proxy ./cmd/cloud-monitoring-proxy

test:
	go test -race -mod=readonly ./...

lint:
	golangci-lint run

run:
	go run ./cmd/cloud-monitoring-proxy

container:
	docker buildx build . --pull --load -t '$(IMAGE):$(TAG)'

push: container
	docker push '$(IMAGE):$(TAG)'

clean:
	rm -rf bin/ cloud-monitoring-proxy

# Live end-to-end suite against a deployed proxy (see e2e/README.md for
# prerequisites). ~15 min; add -short to skip the eviction test.
e2e:
	go test -tags e2e -count=1 ./e2e -v -timeout 45m
