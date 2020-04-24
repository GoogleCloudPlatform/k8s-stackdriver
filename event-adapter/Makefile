OUT_DIR = build
PACKAGE = k8s.io/event-adapter
PREFIX ?= staging-k8s.gcr.io
TAG = 1.0

PKG := $(shell find pkg/* -type f)
CMD := $(shell find cmd/* -type f)

build: build/adapter

build/adapter: adapter.go $(PKG) $(CMD)
	go build -mod=vendor -a -o $(OUT_DIR)/adapter adapter.go

docker: build/adapter
	docker build --pull -t ${PREFIX}/event-adapter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/event-adapter:$(TAG)

test:
	go test -mod=vendor ./...

clean:
	rm -rf build apiserver.local.config
