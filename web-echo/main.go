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
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
)

var fromFile = flag.String("from-file", "", "Set the initial response from the contents of a file")

var response = []byte{}
var responseMux sync.Mutex

func main() {
	flag.Parse()
	port := "8080"
	if fromEnv := os.Getenv("PORT"); fromEnv != "" {
		port = fromEnv
	}

	if *fromFile != "" {
		log.Printf("Loading initial response from file: %v", *fromFile)
		content, err := ioutil.ReadFile(*fromFile)
		if err != nil {
			log.Fatal(err)
		}
		setResponse([]byte(content))
	}

	server := http.NewServeMux()
	server.HandleFunc("/response", postResponse)
	server.HandleFunc("/", echo)
	log.Printf("Server listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, server))
}

func setResponse(newResponse []byte) {
	responseMux.Lock()
	response = newResponse
	responseMux.Unlock()
}

func postResponse(w http.ResponseWriter, r *http.Request) {
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
	setResponse(body)
}

func echo(w http.ResponseWriter, r *http.Request) {
	log.Printf("Serving 'get' request: %s", r.URL.Path)
	responseMux.Lock()
	w.Write(response)
	responseMux.Unlock()
}

// [END all]
