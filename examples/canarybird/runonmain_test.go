package main

import (
	"testing"

	"graphics.gd/variant/Callable"
)

type fatalSentinel struct{}

// channelTB forwards test method calls back to the testing goroutine
// via a channel. Fatal/FailNow panic with fatalSentinel to unwind
// the main goroutine body; the test goroutine receives the call and
// invokes t.Fatal (which calls runtime.Goexit on its own goroutine).
type channelTB struct {
	*testing.T
	calls chan<- func(*testing.T)
}

func (c *channelTB) Error(args ...any) {
	c.calls <- func(t *testing.T) { t.Helper(); t.Error(args...) }
}
func (c *channelTB) Errorf(format string, args ...any) {
	c.calls <- func(t *testing.T) { t.Helper(); t.Errorf(format, args...) }
}
func (c *channelTB) Fatal(args ...any) {
	c.calls <- func(t *testing.T) { t.Helper(); t.Fatal(args...) }
	panic(fatalSentinel{})
}
func (c *channelTB) Fatalf(format string, args ...any) {
	c.calls <- func(t *testing.T) { t.Helper(); t.Fatalf(format, args...) }
	panic(fatalSentinel{})
}
func (c *channelTB) FailNow() {
	c.calls <- func(t *testing.T) { t.Helper(); t.FailNow() }
	panic(fatalSentinel{})
}
func (c *channelTB) Fail() {
	c.calls <- func(t *testing.T) { t.Fail() }
}
func (c *channelTB) Log(args ...any) {
	c.calls <- func(t *testing.T) { t.Helper(); t.Log(args...) }
}
func (c *channelTB) Logf(format string, args ...any) {
	c.calls <- func(t *testing.T) { t.Helper(); t.Logf(format, args...) }
}

// runOnMain dispatches fn to the engine's main goroutine via
// Callable.Defer and blocks until it completes. Almost every
// classdb call has to land on the engine thread, so the canarybird
// integration tests wrap their bodies in this helper.
func runOnMain(t *testing.T, fn func(testing.TB)) {
	t.Helper()
	calls := make(chan func(*testing.T))
	Callable.Defer(Callable.New(func() {
		defer close(calls)
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(fatalSentinel); !ok {
					panic(r)
				}
			}
		}()
		fn(&channelTB{T: t, calls: calls})
	}))
	for call := range calls {
		call(t)
	}
}
