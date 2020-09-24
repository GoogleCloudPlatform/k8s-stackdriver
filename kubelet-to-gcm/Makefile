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

OUT_DIR = build
PACKAGE = github.com/GoogleCloudPlatform/k8s-stackdriver/kubelet-to-gcm
PREFIX = staging-k8s.gcr.io
TAG = 1.3.3

# Rules for building the real image for deployment to gcr.io

compile: monitor/kubelet monitor/controller
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -mod=vendor -a -o $(OUT_DIR)/monitor monitor/main/daemon.go

test: monitor/kubelet monitor/controller
	go test -mod=vendor ${PACKAGE}/monitor ${PACKAGE}/monitor/kubelet ${PACKAGE}/monitor/controller ${PACKAGE}/monitor/config -v

go: compile test;

clean:
	rm -rf build

build: go
	docker build --pull -t ${PREFIX}/kubelet-to-gcm:$(TAG) .

docker: build

push: docker
	gcloud docker -- push ${PREFIX}/kubelet-to-gcm:$(TAG)
