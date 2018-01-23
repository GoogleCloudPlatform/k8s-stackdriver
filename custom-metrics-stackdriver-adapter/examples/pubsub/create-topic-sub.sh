#!/bin/bash

gcloud pubsub topics create test-topic
gcloud pubsub subscriptions create test-sub --topic test-topic

