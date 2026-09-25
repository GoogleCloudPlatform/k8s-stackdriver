/*
Copyright 2026 Google Inc.

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

package presets

import "testing"

func TestBundlesParseAndResolve(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("no embedded preset bundles found")
	}
	for _, n := range names {
		b, ok := Get(n)
		if !ok {
			t.Fatalf("Get(%q) not found though listed by Names()", n)
		}
		if len(b.Types) == 0 && len(b.TypePrefixes) == 0 {
			t.Errorf("bundle %q resolves to no metrics", n)
		}
	}
	if _, ok := Get("does-not-exist"); ok {
		t.Error("Get(does-not-exist) reported ok")
	}
}
