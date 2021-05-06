TAG = v0.3.0
PREFIX = gcr.io/google-samples

docker: 
	docker build --pull -t ${PREFIX}/sd-dummy-exporter:$(TAG) .

push: docker
	gcloud docker -- push ${PREFIX}/sd-dummy-exporter:$(TAG)
