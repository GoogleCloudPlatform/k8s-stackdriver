// Package slices contains utility functions to manipulate slices in Golang.
// Please do not add new functions that already exist in Go's stdlib.
package slices

// IsSubset returns true if the first argument is a subset
// of the second.
func IsSubset(a, b []any) bool {
	aCountPerItem := countPerItem(a)
	bCountPerItem := countPerItem(b)
	for aItem, aCount := range aCountPerItem {
		bCount, ok := bCountPerItem[aItem]
		// Value in a doesn't exist in the superset b
		if !ok {
			return false
		}
		// Value occurs more frequently in a than in b slice
		if aCount > bCount {
			return false
		}
	}
	return true
}

// countPerItem returns a map that contains the all elements of the slice
// mapped to their occurrence count.
func countPerItem(a []any) map[any]int {
	count := make(map[any]int)
	for _, item := range a {
		count[item]++
	}
	return count
}
