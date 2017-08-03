OUT_DIR = build
PACKAGE = k8s.io/k8s-stackdriver-adapter
PREFIX = gcr.io/kawych-test
TAG = 1.0

PKG := $(shell find pkg/* -type f)
CMD := $(shell find cmd/* -type f)

deps:
	glide install --strip-vendor

build: build/adapter

build/adapter: adapter.go $(PKG) $(CMD)
	go build -a -o $(OUT_DIR)/adapter adapter.go

docker: build/adapter
	docker build --pull -t ${PREFIX}/k8s-stackdriver-adapter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/k8s-stackdriver-adapter:$(TAG)

test: cmd/provider/provider_test.go $(PKG) $(CMD)
	go test cmd/provider/provider_test.go

clean:
	rm -rf build apiserver.local.config
