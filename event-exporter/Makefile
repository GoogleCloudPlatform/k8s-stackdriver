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

all: container

ENVVAR = GOOS=linux GOARCH=amd64 CGO_ENABLED=0
BINARY_NAME = event-exporter
ALL_ARCH=amd64 arm64
IMAGE_NAME = event-exporter

PREFIX ?= staging-k8s.gcr.io
TAG ?= v0.3.8

IMAGE=$(PREFIX)/$(IMAGE_NAME)

ifdef ARCH
	IMAGE=$(PREFIX)/$(IMAGE_NAME)-$(ARCH)
	BUILD_FLAGS=--platform=linux/$(ARCH)
endif

build: test
	${ENVVAR} go build -mod=vendor -a -o ${BINARY_NAME}

test:
	${ENVVAR} go test -mod=vendor ./...

container: .container

.container:
	docker buildx build . --pull -t $(IMAGE):$(TAG) $(BUILD_FLAGS) --load

.sub-container-%:
	$(MAKE) --no-print-directory ARCH=$* .container

container-multiarch:  $(addprefix .sub-container-,$(ALL_ARCH))

push: build .push

.push: .container
	docker push $(IMAGE):$(TAG)

.sub-push-%:
	$(MAKE) --no-print-directory ARCH=$* .push

push-multiarch: build $(addprefix .sub-push-,$(ALL_ARCH))
	docker manifest create --amend $(IMAGE):$(TAG) $(addsuffix :$(TAG),$(addprefix $(IMAGE)-,$(ALL_ARCH)))
	for arch in $(ALL_ARCH); do \
	  docker manifest annotate --os=linux --arch=$$arch $(IMAGE):$(TAG) $(IMAGE)-$$arch:$(TAG); \
	done
	docker manifest push -p $(IMAGE):$(TAG)

clean:
	rm -rf ${BINARY_NAME}
