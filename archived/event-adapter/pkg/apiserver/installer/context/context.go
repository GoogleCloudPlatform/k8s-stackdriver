/*
Copyright 2017 The Kubernetes Authors.

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

package context

import (
	"k8s.io/apiserver/pkg/endpoints/request"
)

// requestInformation holds the resource and method for a request in the context.
type requestInformation struct {
	resource string
	method   string
}

type requestListInformation struct {
	method string
}

// contextKey is the type of the keys for the context in this file.
// It's private to avoid conflicts across packages.
type contextKey int

const resourceKey contextKey = iota

// WithRequestInformation returns a copy of parent in which the resource and method values are set
func WithRequestInformation(parent request.Context, resource, method string) request.Context {
	return request.WithValue(parent, resourceKey, requestInformation{resource, method})
}

// WithRequestListInformation returns a copy of parent in which the method values is set
func WithRequestListInformation(parent request.Context, method string) request.Context {
	return request.WithValue(parent, resourceKey, requestListInformation{method})
}

// RequestInformationFrom returns resource and method on the ctx
func RequestInformationFrom(ctx request.Context) (resource string, method string, ok bool) {
	requestInfo, ok := ctx.Value(resourceKey).(requestInformation)
	if !ok {
		return "", "", ok
	}
	return requestInfo.resource, requestInfo.method, ok
}

// RequestListInformationFrom returns method on the ctx
func RequestListInformationFrom(ctx request.Context) (method string, ok bool) {
	requestInfo, ok := ctx.Value(resourceKey).(requestListInformation)
	if !ok {
		return "", ok
	}
	return requestInfo.method, ok
}
