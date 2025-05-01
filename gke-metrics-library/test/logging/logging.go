// Package logging provides helpers to verify logging behavior in tests
package logging

import (
	"errors"
	"fmt"
	"sort"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
)

// TestingT is a helper interface defining the minimal interface from testing.T used by
// this library
// It allows to use the library in both tests and benchmarks
type TestingT interface {
	zaptest.TestingT
}

// Test helps to validate logs in tests
type Test struct {
	TestingT
	Logger *zap.Logger
	Logs   *observer.ObservedLogs
}

// NewTestLogger records all observed log lines and prints them on a failed test
func NewTestLogger(t TestingT, level zapcore.LevelEnabler) Test {
	core, logs := observer.New(level)
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(c zapcore.Core) zapcore.Core { return zapcore.NewTee(c, core) })))
	return Test{TestingT: t, Logger: logger, Logs: logs}
}

// Expect lines starting at log index
type Expect struct {
	// StartIndex defines where in the recorded stream this expectation starts
	StartIndex int
	// Lines expected
	Lines []Line
}

// Line expected in the log
type Line struct {
	Level   zapcore.Level
	Message string
	Fields  []zapcore.Field
}

// Expect verifies all the expected logs are present and correct at the specified indices
// and returns a diff per line not matching expectations
func (t Test) Expect(e Expect) ([]string, error) {
	allLogs := t.Logs.All()
	if len(e.Lines) == 0 {
		if len(allLogs) != 0 {
			return nil, fmt.Errorf("log contains %d entries, want 0", len(allLogs))
		}
		return nil, nil
	}

	if len(allLogs) <= e.StartIndex {
		return nil, fmt.Errorf("log contains %d entries, want at least %d", len(allLogs), e.StartIndex+1)
	}
	logs := allLogs[e.StartIndex:]
	if len(logs) < len(e.Lines) {
		return nil, fmt.Errorf("log contains %d entries, want at least %d", len(logs), len(e.Lines))
	}

	var diffs []string
	for i, line := range e.Lines {
		if diff := CompareLog(t.TestingT, i, logs[i], line); diff != "" {
			diffs = append(diffs, diff)
		}
	}

	return diffs, nil
}

// CompareLog checks if a provided LoggedEntry matches expectations
// It only compares the level, message and fields
// If an entry contains errors that can not be fully specified at test time
// it is possible to use cmpopts.AnyError
func CompareLog(t TestingT, pos int, got observer.LoggedEntry, wantLine Line) string {
	gotLine := Line{Level: got.Level, Message: got.Message, Fields: got.Context}

	return cmp.Diff(wantLine, gotLine, equateErrorsByTypeOrText(), sortLogFields())
}

// equateErrorsByTypeOrText still allows errors to pass the check if their resulting
// Error() strings match
func equateErrorsByTypeOrText() cmp.Option {
	return cmp.FilterValues(filterErrors, cmp.Comparer(compareErrors))
}

func filterErrors(x, y any) bool {
	_, xIsErr := x.(error)
	_, yIsErr := y.(error)
	return xIsErr && yIsErr
}

func compareErrors(x, y any) bool {
	xe := x.(error)
	ye := y.(error)

	if errors.Is(xe, ye) || errors.Is(ye, xe) {
		return true
	}

	return xe.Error() == ye.Error()
}

// sortLogFields by the field type to avoid errors where the order of fields
// does not match
func sortLogFields() cmp.Option {
	return cmp.Transformer("sortLogFields", func(unsorted []zap.Field) []zap.Field {
		sorted := make([]zap.Field, len(unsorted))
		copy(sorted, unsorted)
		sort.Slice(sorted, func(i int, j int) bool {
			return sorted[i].Type > sorted[j].Type
		})
		return sorted
	})
}
