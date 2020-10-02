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

PREFIX = staging-k8s.gcr.io
IMAGE_NAME = event-exporter
TAG = v0.3.4

build:
	${ENVVAR} go build -mod=vendor -a -o ${BINARY_NAME}

test:
	${ENVVAR} go test -mod=vendor ./...

container:
	docker build --pull -t ${PREFIX}/${IMAGE_NAME}:${TAG} .

push: container
	gcloud docker -- push ${PREFIX}/${IMAGE_NAME}:${TAG}

clean:
	rm -rf ${BINARY_NAME}
