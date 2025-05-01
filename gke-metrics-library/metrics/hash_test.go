package metrics

import (
	"testing"
)

func TestHashLabelsCollision(t *testing.T) {
	testCases := []struct {
		desc        string
		labels1     map[string]string
		labels2     map[string]string
		expectEqual bool
	}{
		{
			desc:        "nil labels",
			labels1:     nil,
			labels2:     nil,
			expectEqual: true,
		},
		{
			desc:        "nil and non-nil label",
			labels1:     nil,
			labels2:     testLabels,
			expectEqual: false,
		},
		{
			desc:        "same labels",
			labels1:     testLabels,
			labels2:     testLabels,
			expectEqual: true,
		},
		{
			desc: "same labels but different order in map",
			labels1: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			labels2: map[string]string{
				"key2": "value2",
				"key1": "value1",
			},
			expectEqual: true,
		},
		{
			desc:        "different labels",
			labels1:     testLabels,
			labels2:     testLabels2,
			expectEqual: false,
		},
		{
			desc: "same key different value",
			labels1: map[string]string{
				"key1": "value1",
			},
			labels2: map[string]string{
				"key1": "value2",
			},
			expectEqual: false,
		},
		{
			desc: "same value different key",
			labels1: map[string]string{
				"key1": "value1",
			},
			labels2: map[string]string{
				"key2": "value1",
			},
			expectEqual: false,
		},
		{
			desc: "either key or value is empty",
			labels1: map[string]string{
				"key1": "",
			},
			labels2: map[string]string{
				"": "key1",
			},
			expectEqual: false,
		},
		{
			desc: "should have separator between 2 key/value pairs",
			labels1: map[string]string{
				"":     "key1",
				"key2": "",
			},
			labels2: map[string]string{
				"":         "",
				"key1key2": "",
			},
			expectEqual: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			id1 := hashLabels(tc.labels1)
			id2 := hashLabels(tc.labels2)
			if (id1 == id2) != tc.expectEqual {
				t.Errorf("hashLabels(%v) = %v, hashLabels(%v) = %v, expected to be the same: %v", tc.labels1, id1, tc.labels2, id2, tc.expectEqual)
			}
		})
	}
}
