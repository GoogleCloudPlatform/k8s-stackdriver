package metrics

import "github.com/OneOfOne/xxhash"

// emptyLabelsHash is the hash of an empty set of labels.
// It is used in hashLabels to avoid computing the hash in the common case
// where there are no labels.
var emptyLabelsHash uint64

func init() {
	// Initialize emptyLabelsHash.
	emptyLabelsHash = xxhash.New64().Sum64()
}

// hashLabels returns a unique uint64 for the given set of key-value labels.
//
//go:nosplit
func hashLabels(labels map[string]string) uint64 {
	if len(labels) == 0 {
		return emptyLabelsHash
	}
	// Because Go map iteration is in random order, we cannot hash all of the
	// map contents as we iterate through them.
	// While it would be possible to allocate a []string slice to order the keys
	// and then iterate through them in sorted order, doing so allocates a new
	// slice on the heap for every single call to `WithLabels`, which can be
	// very expensive on GC churn.
	// Instead, this approach computes the XOR of the hashes of each key-value
	// pair, which means it is independent of map iteration order. It also
	// avoids doing any heap allocation.
	xxh := xxhash.New64()
	var hash uint64
	for k, v := range labels {
		xxh.Reset()
		// NOGOUNUSED-START
		xxh.WriteString(k)
		xxh.WriteString(labelSeparator)
		xxh.WriteString(v)
		// NOGOUNUSED-END
		hash ^= xxh.Sum64()
	}
	return hash
}
