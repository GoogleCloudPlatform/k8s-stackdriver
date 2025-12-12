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
IMAGE_NAME = kubelet-to-gcm
PREFIX ?= staging-k8s.gcr.io
TAG ?= 1.4.4
ALL_ARCH=amd64 arm64

IMAGE=$(PREFIX)/$(IMAGE_NAME)

ifdef ARCH
	TAG=$(TAG)-$(ARCH)
	BUILD_FLAGS=--platform=linux/$(ARCH)
endif

# Rules for building the real image for deployment to gcr.io

compile: monitor/kubelet monitor/controller
	GOOS=$(if $(GOOS),$(GOOS),linux) GOARCH=$(if $(GOARCH),$(GOARCH),amd64) CGO_ENABLED=0 go build -mod=vendor -a -o $(OUT_DIR)/monitor monitor/main/daemon.go

test: monitor/kubelet monitor/controller
	go test -mod=vendor ${PACKAGE}/monitor ${PACKAGE}/monitor/kubelet ${PACKAGE}/monitor/controller ${PACKAGE}/monitor/config -v

go: compile test;

clean:
	rm -rf build

.container:
	docker buildx build . --pull -t $(IMAGE):$(TAG) $(BUILD_FLAGS) --load

.sub-container-%:
	$(MAKE) --no-print-directory ARCH=$* .container

container-multiarch:  $(addprefix .sub-container-,$(ALL_ARCH))

.push: .container
	docker push $(IMAGE):$(TAG)

push: .push

.sub-push-%:
	$(MAKE) --no-print-directory ARCH=$* .push

push-multiarch: $(addprefix .sub-push-,$(ALL_ARCH))
	docker manifest create --amend $(IMAGE):$(TAG) $(addprefix $(IMAGE):$(TAG)-, $(ALL_ARCH))
	for arch in $(ALL_ARCH); do \
	  docker manifest annotate --os=linux --arch=$$arch $(IMAGE):$(TAG) $(IMAGE):$(TAG)-$$arch; \
	done
	docker manifest push -p $(IMAGE):$(TAG)

clean:
	rm -rf monitor

coverage:
	rm -f coverage.out
	go test -coverpkg=./... -coverprofile=coverage.out ./...
