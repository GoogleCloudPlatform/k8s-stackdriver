/*
Copyright 2017 Google Inc.

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

package translator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v3 "google.golang.org/api/monitoring/v3"
)

var equalDescriptor = "equal"
var differentDescription = "differentDescription"
var differentLabels = "differentLabels"

var description1 = "Simple description"
var description2 = "Complex description"

var label1 = &v3.LabelDescriptor{Key: "label1"}
var label2 = &v3.LabelDescriptor{Key: "label2"}
var label3 = &v3.LabelDescriptor{Key: "label3"}

var originalDescriptor = v3.MetricDescriptor{
	Name:        equalDescriptor,
	Description: description1,
	Labels:      []*v3.LabelDescriptor{label1, label2},
}

var otherDescriptors = map[*v3.MetricDescriptor]bool{
	{
		Name:        equalDescriptor,
		Description: description1,
		Labels:      []*v3.LabelDescriptor{label1, label2},
	}: false,
	{
		Name:        differentDescription,
		Description: description2,
		Labels:      []*v3.LabelDescriptor{label1, label2},
	}: true,
	{
		Name:        differentLabels,
		Description: description1,
		Labels:      []*v3.LabelDescriptor{label3},
	}: true,
}

func TestDescriptorChanged(t *testing.T) {
	for descriptor, result := range otherDescriptors {
		if result {
			assert.True(t, descriptorChanged(&originalDescriptor, descriptor))
		} else {
			assert.False(t, descriptorChanged(&originalDescriptor, descriptor))
		}
	}
}
