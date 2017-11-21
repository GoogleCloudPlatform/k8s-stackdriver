/**
 * Copyright 2017 Google Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// [START all]
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

var response []byte
var response_mux sync.Mutex

func main() {
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}

	set_response([]byte(""))

	server := http.NewServeMux()
	server.HandleFunc("/response", put_response)
	server.HandleFunc("/", echo)
	log.Printf("Server listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, server))
}

func set_response(new_response []byte) {
	response_mux.Lock()
	response = append([]byte("# HELP process_start_time_seconds Start time of the process since unix epoch in seconds.\n"+
		"# TYPE process_start_time_seconds gauge\n"+
		fmt.Sprintf("process_start_time_seconds %v\n", time.Now().Unix())),
		new_response...)
	response_mux.Unlock()
}

func put_response(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		echo(w, r)
		return
	}
	log.Printf("Serving 'set' request: %s", r.URL.Path)
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Cannot read body from request: %v", err), http.StatusBadRequest)
	}
	log.Printf("Setting response: %s", body)
	set_response(body)
}

func echo(w http.ResponseWriter, r *http.Request) {
	log.Printf("Serving 'get' request: %s", r.URL.Path)
	response_mux.Lock()
	w.Write(response)
	response_mux.Unlock()
}

// [END all]
