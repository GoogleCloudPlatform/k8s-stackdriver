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
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/golang/glog"
	"golang.org/x/net/context"
)

func main() {
	flag.Parse()

	glog.V(1).Infof("Starting subscriber")

	ctx := context.Background()

	// [START auth]
	creds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if creds == "" {
		glog.Fatalf("GOOGLE_APPLICATION_CREDENTIALS environment variable must be set.\n")
	}
	glog.Infof("Using credentials from %v", creds)

	proj := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if proj == "" {
		glog.Fatalf("GOOGLE_CLOUD_PROJECT environment variable must be set.\n")
	}
	client, err := pubsub.NewClient(ctx, proj)
	if err != nil {
		glog.Fatalf("Could not create pubsub Client: %v", err)
	}
	// [END auth]

	// Get subscription.
	subName := os.Getenv("SUBSCRIPTION")
	if proj == "" {
		glog.Fatalf("SUBSCRIPTION environment variable must be set.\n")
	}

	interval := os.Getenv("INTERVAL")
	if interval == "" {
		glog.Fatalf("INTERVAL environment variable must be set.\n")
	}
	intervalDuration, err := time.ParseDuration(interval)
	if err != nil {
		glog.Fatalf("Cannot parse INTERVAL duration: %s", interval)
	}

	// Pull messages via the subscription.
	var mu sync.Mutex
	received := 0
	for {
		sub := client.Subscription(subName)
		sub.ReceiveSettings.MaxOutstandingMessages = 1
		cctx, _ := context.WithCancel(ctx)
		err := sub.Receive(cctx, func(ctx context.Context, msg *pubsub.Message) {
			mu.Lock()
			defer mu.Unlock()
			received++
			fmt.Println("Got message: ", string(msg.Data))
			msg.Ack()
			time.Sleep(intervalDuration)
		})
		if err != nil {
			glog.Errorf("Error pulling messages: %v", err)
		}
	}
}
