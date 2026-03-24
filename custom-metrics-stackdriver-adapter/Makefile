ARCH?=amd64
GOOS?=linux
OUT_DIR?=build
PACKAGE=github.com/GoogleCloudPlatform/k8s-stackdriver/custom-metrics-stackdriver-adapter
PREFIX?=staging-k8s.gcr.io
TAG = v0.16.5
PKG := $(shell find pkg/* -type f)

.PHONY: build docker push test clean verify-generated update-generated

build: build/adapter

build/adapter: adapter.go $(PKG)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(ARCH) go build -mod readonly -a -o $(OUT_DIR)/$(ARCH)/adapter adapter.go

docker:
	docker buildx build . --pull --load -t ${PREFIX}/custom-metrics-stackdriver-adapter:$(TAG)

push: docker
	gcloud docker -- push ${PREFIX}/custom-metrics-stackdriver-adapter:$(TAG)

test: $(PKG)
	CGO_ENABLED=0 go test -mod readonly ./...

clean:
	rm -rf build

coverage:
	rm -f coverage.out
	go test -coverpkg=./... -coverprofile=coverage.out ./...

# Generation
# ----------

generated_files=pkg/api/generated/openapi/zz_generated.openapi.go

verify-generated: update-generated
	@git diff --exit-code -- $(generated_files)

update-generated:
	go run -mod=readonly k8s.io/kube-openapi/cmd/openapi-gen \
	  --logtostderr \
	  --go-header-file ./hack/boilerplate.go.txt \
	  --output-pkg ./pkg/api/generated/openapi \
	  --output-file zz_generated.openapi.go \
	  --output-dir ./pkg/api/generated/openapi \
	  -r /dev/null \
	  "k8s.io/metrics/pkg/apis/custom_metrics" \
	  "k8s.io/metrics/pkg/apis/custom_metrics/v1beta1" \
	  "k8s.io/metrics/pkg/apis/custom_metrics/v1beta2" \
	  "k8s.io/metrics/pkg/apis/external_metrics" \
	  "k8s.io/metrics/pkg/apis/external_metrics/v1beta1" \
	  "k8s.io/metrics/pkg/apis/metrics" \
	  "k8s.io/metrics/pkg/apis/metrics/v1beta1" \
	  "k8s.io/apimachinery/pkg/apis/meta/v1" \
	  "k8s.io/apimachinery/pkg/api/resource" \
	  "k8s.io/api/core/v1" \
	  "k8s.io/apimachinery/pkg/version"
