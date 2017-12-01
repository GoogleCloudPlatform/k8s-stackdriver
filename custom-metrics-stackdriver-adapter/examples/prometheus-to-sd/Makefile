TAG = v0.1.0
PREFIX = gcr.io/google-containers

build: prometheus_dummy_exporter

prometheus_dummy_exporter: prometheus_dummy_exporter.go
	go build -a -o prometheus_dummy_exporter prometheus_dummy_exporter.go

docker: prometheus_dummy_exporter
	docker build --pull -t ${PREFIX}/prometheus-dummy-exporter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/prometheus-dummy-exporter:$(TAG)

clean:
	rm -rf prometheus_dummy_exporter
