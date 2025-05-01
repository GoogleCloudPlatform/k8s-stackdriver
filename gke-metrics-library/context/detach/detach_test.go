package detach

import (
	"context"
	"testing"
	"time"
)

type value struct {
}

type ctxKey struct {
	comment string
}

var (
	// ignoredKey does not have a PreserveFunc registered.
	ignoredKey = &ctxKey{"detach.ignoredKey"}
	// preservedKey preserves copies of its values in detached Contexts.
	preservedKey = &ctxKey{"detach.preservedKey"}
)

func TestIgnoreCancel(t *testing.T) {
	parentIgnoredVal := &value{}
	parentPreservedVal := &value{}
	base := context.WithValue(context.WithValue(context.Background(), ignoredKey, parentIgnoredVal), preservedKey, parentPreservedVal)
	cases := []struct {
		name   string
		parent func() context.Context
	}{
		{
			name:   "{ignoredKey: &value{}, preservedKey: &value{}}",
			parent: func() context.Context { return base },
		},
		{
			name: "context.WithCancel({ignoredKey: &value{}, preservedKey: &value{}})",
			parent: func() context.Context {
				ctx, _ := context.WithCancel(base)
				return ctx
			},
		},
		{
			name: "canceled {ignoredKey: &value{}, preservedKey: &value{}}",
			parent: func() context.Context {
				ctx, cancel := context.WithCancel(base)
				cancel()
				return ctx
			},
		},
		{
			name: "context.WithDeadline({ignoredKey: &value{}, preservedKey: &value{}}, now + 1 year)",
			parent: func() context.Context {
				ctx, _ := context.WithDeadline(base, time.Now().Add(365*24*time.Hour))
				return ctx
			},
		},
		{
			name: "context.WithDeadline({ignoredKey: &value{}, preservedKey: &value{}}, now - 5 minutes)",
			parent: func() context.Context {
				ctx, _ := context.WithDeadline(base, time.Now().Add(-5*time.Minute))
				return ctx
			},
		},
	}

	for _, tc := range cases {
		child := IgnoreCancel(tc.parent())
		if d, hasDeadline := child.Deadline(); hasDeadline {
			t.Errorf("detach.IgnoreCancel(%s).Deadline() = %v, true; want _, false", tc.name, d)
		}
		if child.Done() != nil {
			t.Errorf("detach.IgnoreCancel(%s).Done() != nil", tc.name)
		}
		if child.Err() != nil {
			t.Errorf("detach.IgnoreCancel(%s).Err() != nil", tc.name)
		}
		if v, ok := child.Value(ignoredKey).(*value); !ok || v != parentIgnoredVal {
			t.Errorf("detach.IgnoreCancel(%s).Value(ignoredKey) = %p; want %p", tc.name, v, parentIgnoredVal)
		}
		if v, ok := child.Value(preservedKey).(*value); !ok || v != parentPreservedVal {
			t.Errorf("detach.IgnoreCancel(%s).Value(preservedKey) = %p; want %p", tc.name, v, parentPreservedVal)
		}
	}
}
