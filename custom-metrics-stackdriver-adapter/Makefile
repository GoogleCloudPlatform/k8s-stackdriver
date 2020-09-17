ARCH?=amd64
GOOS?=linux
OUT_DIR?=build
PACKAGE=github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter
PREFIX?=staging-k8s.gcr.io
TAG = v0.11.3
PKG := $(shell find pkg/* -type f)

.PHONY: build docker push test clean

build: build/adapter

build/adapter: adapter.go $(PKG)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(ARCH) go build -mod vendor -a -o $(OUT_DIR)/$(ARCH)/adapter adapter.go

docker:
	docker build --pull -t ${PREFIX}/custom-metrics-stackdriver-adapter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/custom-metrics-stackdriver-adapter:$(TAG)

test: $(PKG)
	CGO_ENABLED=0 go test -mod vendor ./...

clean:
	rm -rf build

