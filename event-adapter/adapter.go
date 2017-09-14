/*
Copyright 2016 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
		http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"runtime"

	"github.com/GoogleCloudPlatform/k8s-stackdriver/event-adapter/cmd/server"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/util/logs"
)

var (
	maxRetrievedEvents = flag.Int("max-retrieved-events", 100, "Maximum number of events can be retrieved")
	sinceMillis        = flag.Int64("retrieve-events-since-millis", -1, "Retrieve only events logged after given timestamp since epoch, if the given timestamp is < 0 the default of last 1h will be used")
)

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()
	flag.Parse()
	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
	cmd := server.NewCommandStartSampleAdapterServer(os.Stdout, os.Stderr, wait.NeverStop, *maxRetrievedEvents, *sinceMillis)
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
