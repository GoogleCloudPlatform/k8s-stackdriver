OUT_DIR = build
PACKAGE = github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter
PREFIX = gcr.io/google-containers
TAG = v1.0

PKG := $(shell find pkg/* -type f)

deps:
	glide install --strip-vendor

build: build/adapter

build/adapter: sample-main.go $(PKG)
	go build -a -o $(OUT_DIR)/adapter sample-main.go

docker: build/adapter
	docker build --pull -t ${PREFIX}/custom-metrics-stackdriver-adapter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/custom-metrics-stackdriver-adapter:$(TAG)

clean:
	rm -rf build apiserver.local.config
