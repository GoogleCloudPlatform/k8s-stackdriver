// Package detach contains only the necessary functions from "google3/go/context/detach". There is no good OSS package to use for now.
// In go 1.21, the standard library "context" contains a WithoutCancel() method which has the same functionality.
// Remove this copy once we update to go 1.21 and replace with standard library.
package detach

import (
	"context"
	"fmt"
	"time"
)

type ignoreCtx struct {
	parent context.Context
}

func (c ignoreCtx) Value(key any) any                       { return c.parent.Value(key) }
func (c ignoreCtx) Deadline() (deadline time.Time, ok bool) { return }
func (c ignoreCtx) Done() <-chan struct{}                   { return nil }
func (c ignoreCtx) Err() error                              { return nil }

func (c ignoreCtx) String() string {
	return fmt.Sprintf("detach.IgnoreCancel(%v)", c.parent)
}

// IgnoreCancel returns a new Context that takes its values from parent but
// ignores any cancelation or deadline on parent.
//
// IgnoreCancel must only be used synchronously.  To detach a Context for use in
// an asynchronous API, use Go instead.
//
// You can create fail-safe cleanup functions in case the parent context is
// cancelled. Example:
//
//	func New(ctx context.Context, id string) (*Publisher, error) {
//	  client := buildClient(ctx)
//
//	  if err := validate(id); err != nil {
//	    client.Close(detach.IgnoreCancel(ctx))
//	    return nil, err
//	  }
//
//	  return &Publisher{client: client}, nil
//	}
func IgnoreCancel(parent context.Context) context.Context {
	return ignoreCtx{parent}
}
