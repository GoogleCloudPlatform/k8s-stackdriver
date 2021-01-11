TAG = v0.2.1
PREFIX = staging-k8s.gcr.io

build: sd_dummy_exporter

sd_dummy_exporter: sd_dummy_exporter.go
	go build -a -o sd_dummy_exporter sd_dummy_exporter.go

docker: sd_dummy_exporter
	docker build --pull -t ${PREFIX}/sd-dummy-exporter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/sd-dummy-exporter:$(TAG)

clean:
	rm -rf sd_dummy_exporter
