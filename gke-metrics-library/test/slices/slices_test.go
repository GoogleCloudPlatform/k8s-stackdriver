package slices

import (
	"testing"
)

func TestIsSubset(t *testing.T) {
	testCases := []struct {
		desc     string
		a        []any
		b        []any
		isSubset bool
	}{
		{
			desc:     "int slice without duplicates is a subset of a larger int slice",
			a:        []any{1, 2, 3},
			b:        []any{1, 2, 3, 4},
			isSubset: true,
		},
		{
			desc:     "non-empty slice is a subset of itself",
			a:        []any{1, 2, 3},
			b:        []any{1, 2, 3},
			isSubset: true,
		},
		{
			desc:     "empty slices is a subset of itself",
			a:        []any{},
			b:        []any{},
			isSubset: true,
		},
		{
			desc:     "Slices with duplicates is a subset of another slice with duplicates",
			a:        []any{1, 1, 2},
			b:        []any{1, 1, 1, 2, 3, 4},
			isSubset: true,
		},
		{
			desc:     "float64 slice is a subset of a larger float64 slice",
			a:        []any{1.1, 2.2},
			b:        []any{1.1, 1.1, 2.2, 3.3, 4.4},
			isSubset: true,
		},
		{
			desc:     "empty slice is a subset of a non-empty slice",
			a:        []any{},
			b:        []any{1.1, 1.1, 2.2, 3.3, 4.4},
			isSubset: true,
		},
		// Negative test cases:
		{
			desc:     "different int slice is not a subset of another int slice",
			a:        []any{1, 2, 3},
			b:        []any{4, 5, 6},
			isSubset: false,
		},
		{
			desc:     "int slice is not a subset of a smaller int slice",
			a:        []any{1, 2, 3, 4},
			b:        []any{1, 2, 3},
			isSubset: false,
		},
		{
			desc:     "non-empty slice is not a subset of an empty slice",
			a:        []any{1, 2, 3},
			b:        []any{},
			isSubset: false,
		},
		{
			desc:     "Slice with duplicates is not a subset of a slice without duplicates",
			a:        []any{1, 1, 2, 3},
			b:        []any{1, 2, 3},
			isSubset: false,
		},
		{
			desc:     "float64 slice is not a subset of an int slice",
			a:        []any{1.1, 2.2},
			b:        []any{1, 2},
			isSubset: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if got := IsSubset(tc.a, tc.b); got != tc.isSubset {
				t.Errorf("IsSubset(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.isSubset)
			}
		})
	}
}
