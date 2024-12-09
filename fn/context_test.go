package fn

import (
	"context"
	"testing"
	"time"
)

// TestContextWithQuit tests the behaviour of the ContextWithQuit function.
// We test that the derived context is correctly cancelled when either the
// passed context is cancelled or the quit channel is closed.
func TestContextWithQuit(t *testing.T) {
	t.Parallel()

	// Test that the derived context is cancelled when the passed context is
	// cancelled.
	t.Run("Parent context is cancelled", func(t *testing.T) {
		t.Parallel()

		var (
			ctx, cancel = context.WithCancel(context.Background())
			quit        = make(chan struct{})
		)

		ctxc, _ := ContextWithQuit(ctx, quit)

		// Cancel the parent context.
		cancel()

		// Assert that the derived context is cancelled.
		select {
		case <-ctxc.Done():
		default:
			t.Errorf("The derived context should be cancelled at " +
				"this point")
		}
	})

	// Test that the derived context is cancelled when the passed quit
	// channel is closed.
	t.Run("Quit channel is closed", func(t *testing.T) {
		var (
			ctx  = context.Background()
			quit = make(chan struct{})
		)

		ctxc, _ := ContextWithQuit(ctx, quit)

		// Close the quit channel.
		close(quit)

		// Assert that the derived context is cancelled.
		select {
		case <-ctxc.Done():
		default:
			t.Errorf("The derived context should be cancelled at " +
				"this point")
		}
	})

	t.Run("Parent context is already closed", func(t *testing.T) {
		var (
			ctx, cancel = context.WithCancel(context.Background())
			quit        = make(chan struct{})
		)

		cancel()

		ctxc, _ := ContextWithQuit(ctx, quit)

		// Assert that the derived context is cancelled already
		// cancelled.
		select {
		case <-ctxc.Done():
		default:
			t.Errorf("The derived context should be cancelled at " +
				"this point")
		}
	})

	t.Run("Passed quit channel is already closed", func(t *testing.T) {
		var (
			ctx  = context.Background()
			quit = make(chan struct{})
		)

		close(quit)

		ctxc, _ := ContextWithQuit(ctx, quit)

		// Assert that the derived context is cancelled already
		// cancelled.
		select {
		case <-ctxc.Done():
		default:
			t.Errorf("The derived context should be cancelled at " +
				"this point")
		}
	})

	t.Run("Child context should be cancelled synchronously with "+
		"parent", func(t *testing.T) {

		var (
			ctx, cancel = context.WithCancel(context.Background())
			quit        = make(chan struct{})
			task        = make(chan struct{})
			done        = make(chan struct{})
		)

		// Derive a child context.
		ctxc, _ := ContextWithQuit(ctx, quit)

		// Spin off a routine that exists cleaning if the child context
		// is cancelled but fails if the task is performed.
		go func() {
			defer close(done)
			select {
			case <-ctxc.Done():
			case <-task:
				t.Fatalf("should not get here")
			}
		}()

		// Give the goroutine some time to spin up.
		time.Sleep(time.Millisecond * 500)

		// First cancel the parent context. Then immediately execute the
		// task.
		cancel()
		close(task)

		// Wait for the goroutine to exit.
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("timeout")
		}
	})

	t.Run("Child context should be cancelled synchronously with the "+
		"close of the quit channel", func(t *testing.T) {

		var (
			ctx  = context.Background()
			quit = make(chan struct{})
			task = make(chan struct{})
			done = make(chan struct{})
		)

		// Derive a child context.
		ctxc, _ := ContextWithQuit(ctx, quit)

		// Spin off a routine that exists cleaning if the child context
		// is cancelled but fails if the task is performed.
		go func() {
			defer close(done)
			select {
			case <-ctxc.Done():
			case <-task:
				t.Fatalf("should not get here")
			}
		}()

		// First cancel the parent context. Then immediately execute the
		// task.
		close(quit)

		// Give the context some time to cancel the derived context.
		time.Sleep(time.Millisecond * 500)

		close(task)

		// Wait for the goroutine to exit.
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("timeout")
		}
	})
}
