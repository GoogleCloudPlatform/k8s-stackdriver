// Package logging provides reliable logging in GKE Metrics.
package logging

import (
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// RateLimitedLogger provides a wrapped logger with a sampling policy that logs the first N
// duplicate entries with a given level and message per sampling interval.
// If maxDuplicates < 0, no rate limit will be applied.
func RateLimitedLogger(logger *zap.Logger, samplingInterval time.Duration, maxDuplicates int) *zap.Logger {
	if maxDuplicates < 0 {
		return logger
	}
	opts := zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewSamplerWithOptions(core, samplingInterval, maxDuplicates, 0)
	})
	return logger.WithOptions(opts)
}
