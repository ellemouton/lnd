package fn

import (
	"context"
)

// ctxWithQuit is a context that is canceled when either the passed context is
// canceled or the quit channel is closed.
type ctxWithQuit struct {
	context.Context

	// quit is the quit channel that was passed to ContextWithQuit. This is
	// the caller's quit channel and is not ours to close. It should only
	// ever be listened on.
	quit <-chan struct{}
}

// Done returns a channel that is closed when either the passed context is
// canceled or the quit channel is closed.
//
// NOTE: this is part of the context.Context interface.
func (c *ctxWithQuit) Done() <-chan struct{} {
	select {
	case <-c.quit:
		return c.quit
	default:
		return c.Context.Done()
	}
}

// Err returns the error that caused the context to be canceled. If the passed
// context is canceled, then the error from the passed context is returned.
// Otherwise, if the quit channel is closed, then context.Canceled is returned.
//
// NOTE: this is part of the context.Context interface.
func (c *ctxWithQuit) Err() error {
	select {
	case <-c.Context.Done():
		return c.Context.Err()
	case <-c.quit:
		return context.Canceled
	default:
		return nil
	}
}

// ContextWithQuit returns a new context that is canceled when either the
// passed context is canceled or the quit channel is closed. This in essence
// combines the signals of the passed context and the quit channel.
//
// NOTE: if the parent context is canceled first, then the returned context is
// guaranteed to be cancelled immediately since it is derived from the parent.
// However, if the quit channel is closed first, then there the returned context
// will be closed once the closed quit channel signal has been responded to.
func ContextWithQuit(ctx context.Context,
	quit <-chan struct{}) (context.Context, context.CancelFunc) {

	// Derive a fresh context from the passed context. If the passed
	// context has already been canceled, then this fresh one will also
	// already be canceled.
	ctx, cancel := context.WithCancel(ctx)

	select {
	case <-ctx.Done():
		return ctx, cancel
	case <-quit:
		cancel()
		return ctx, cancel
	default:
	}

	go func() {
		select {
		// If the derived context is canceled, which will be the case
		// if either the passed parent context is canceled or if the
		// returned cancel function is called, then there is nothing
		// left to do.
		case <-ctx.Done():

		// If the quit channel is closed, then we cancel the derived
		// context which was returned.
		case <-quit:
			cancel()
		}
	}()

	return &ctxWithQuit{Context: ctx, quit: quit}, cancel
}
