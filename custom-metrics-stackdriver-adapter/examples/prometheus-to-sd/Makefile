TAG = v0.3.0
PREFIX = gcr.io/google-samples

docker:
	docker build --pull -t ${PREFIX}/prometheus-dummy-exporter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/prometheus-dummy-exporter:$(TAG)
