TAG ?= dev

build: subscriber

subscriber: subscriber.go
	go get cloud.google.com/go/pubsub
	go build -a -o subscriber subscriber.go

docker: subscriber
	docker build --pull -t ${REGISTRY}/pubsub-subscriber:$(TAG) .

push: docker
	gcloud docker -- push ${REGISTRY}/pubsub-subscriber:$(TAG)

clean:
	rm -rf subscriber
