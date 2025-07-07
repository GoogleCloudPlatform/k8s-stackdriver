// Copyright 2018 Google Inc. All rights reserved.
// Use of this source code is governed by the Apache 2.0
// license that can be found in the LICENSE file.

// Command subscriptions is a tool to manage Google Cloud Pub/Sub subscriptions by using the Pub/Sub API.
// See more about Google Cloud Pub/Sub at https://cloud.google.com/pubsub/docs/overview.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	// [START imports]

	"cloud.google.com/go/pubsub"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	// [END imports]
)

// Get env variable or die.
func getenv(name string) string {
	res := os.Getenv(name)
	if res == "" {
		glog.Fatalf("%s environment variable must be set.\n", name)
	}
	return res
}

func main() {
	flag.Parse()

	glog.V(1).Infof("Starting publisher")

	ctx := context.Background()

	// Read config.
	creds := getenv("GOOGLE_APPLICATION_CREDENTIALS")
	proj := getenv("GOOGLE_CLOUD_PROJECT")
	topic := getenv("TOPIC")
	podID := getenv("POD_ID")
	interval := getenv("INTERVAL")
	intervalDuration, err := time.ParseDuration(interval)
	if err != nil {
		glog.Fatalf("Cannot parse INTERVAL duration: %s", interval)
	}

	// Create client.
	glog.Infof("Using credentials from: %v", creds)
	client, err := pubsub.NewClient(ctx, proj)
	if err != nil {
		glog.Fatalf("Could not create pubsub Client: %v", err)
	}

	// Get topic.
	t := client.Topic(topic)

	// Start publishing messages.
	for i := 0; true; i++ {
		message := fmt.Sprintf("%s says hello world #%d", podID, i)
		res := t.Publish(ctx, &pubsub.Message{
			Data: []byte(message),
		})
		fmt.Println("Published message ", message)
		_, err := res.Get(ctx)
		if err != nil {
			glog.Errorf("Error publishing message %v: %v", message, err)
		}
		time.Sleep(intervalDuration)
	}
}
