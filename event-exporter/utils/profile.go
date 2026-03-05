package utils

import (
	"bytes"
	"net/http"
	"runtime/pprof"

	"k8s.io/klog/v2"
)

func TakeMemorySnapshot() {
	var capturedProfile bytes.Buffer
	// Write the profile into the RAM buffer instead of a file
	if err := pprof.WriteHeapProfile(&capturedProfile); err != nil {
		klog.Errorf("could not write memory profile: %v", err)
		return
	}
	klog.Info("✅ Memory snapshot successfully captured in RAM!")

	// Serve that exact snapshot over a custom HTTP endpoint
	http.HandleFunc("/snapshot", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="memprofile.pprof"`)
		w.Write(capturedProfile.Bytes())
	})

	// Start a lightweight HTTP server just for this file (if you don't already have one running)
	// You can use a different port if 6060 is taken.
	go func() {
		klog.Info("Serving snapshot on localhost:6062/snapshot")
		if err := http.ListenAndServe("localhost:6062", nil); err != nil {
			klog.Errorf("snapshot server failed: %v", err)
		}
	}()
}
