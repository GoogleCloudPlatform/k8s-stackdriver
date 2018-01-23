#!/bin/bash

PROJECT=$1

function create-secret {
  gcloud iam service-accounts create pub-sub-$1 --display-name "Pub/Sub $1" --project $PROJECT
  gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:pub-sub-$1@$PROJECT.iam.gserviceaccount.com --role roles/pubsub.$1
  gcloud projects add-iam-policy-binding $PROJECT --member serviceAccount:pub-sub-$1@$PROJECT.iam.gserviceaccount.com --role roles/pubsub.viewer
  gcloud iam service-accounts keys create /tmp/pub-sub-$1-key.json --iam-account  pub-sub-$1@$PROJECT.iam.gserviceaccount.com
  kubectl create secret generic pubsub-$1-key --from-file=key.json=/tmp/pub-sub-$1-key.json
}

create-secret publisher
create-secret subscriber

