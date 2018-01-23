TAG ?= dev

build: publisher

publisher: publisher.go
	go get cloud.google.com/go/pubsub
	go build -a -o publisher publisher.go

docker: publisher
	docker build --pull -t ${REGISTRY}/pubsub-publisher:$(TAG) .

push: docker
	gcloud docker -- push ${REGISTRY}/pubsub-publisher:$(TAG)

clean:
	rm -rf publisher
