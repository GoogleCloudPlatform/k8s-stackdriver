ARCH?=amd64
OUT_DIR?=build
PACKAGE=github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter
PREFIX?=gcr.io/google-containers
TAG = v0.6.0
PKG := $(shell find pkg/* -type f)

.PHONY: build docker push test clean

build: build/adapter

build/adapter: adapter.go $(PKG)
	CGO_ENABLED=0 GOARCH=$(ARCH) go build -a -o $(OUT_DIR)/$(ARCH)/adapter adapter.go

docker: build/adapter
	docker build --pull -t ${PREFIX}/custom-metrics-stackdriver-adapter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/custom-metrics-stackdriver-adapter:$(TAG)

test: $(PKG)
	CGO_ENABLED=0 go test ./pkg/...

clean:
	rm -rf build

