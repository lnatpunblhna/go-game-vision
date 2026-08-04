package tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/safety"
)

// newTestGuard builds a Guard with a fast poll interval and no signal handling,
// so tests neither interfere with the test binary's own signals nor wait long.
func newTestGuard(t *testing.T, opts *safety.Options) *safety.Guard {
	t.Helper()

	if opts == nil {
		opts = &safety.Options{}
	}
	opts.DisableSignals = true
	if opts.PollInterval == 0 {
		opts.PollInterval = 5 * time.Millisecond
	}

	g, err := safety.New(opts)
	if err != nil {
		// Corner detection needs the screen size, which a headless environment
		// may not provide.
		t.Skipf("could not arm emergency stop (headless?): %v", err)
	}
	return g
}

func TestGuardStartsUntripped(t *testing.T) {
	g := newTestGuard(t, nil)
	defer g.Close()

	if g.Tripped() {
		t.Error("Guard should not be tripped immediately after New")
	}
	if err := g.Err(); err != nil {
		t.Errorf("Err() = %v, want nil before tripping", err)
	}
	if reason := g.Reason(); reason != "" {
		t.Errorf("Reason() = %q, want empty before tripping", reason)
	}

	select {
	case <-g.Done():
		t.Error("Done() closed before tripping")
	default:
	}
}

func TestGuardTripSetsStateAndCancelsContext(t *testing.T) {
	g := newTestGuard(t, nil)
	defer g.Close()

	g.Trip("test reason")

	if !g.Tripped() {
		t.Error("Tripped() = false after Trip")
	}
	if reason := g.Reason(); reason != "test reason" {
		t.Errorf("Reason() = %q, want %q", reason, "test reason")
	}

	err := g.Err()
	if err == nil {
		t.Fatal("Err() = nil after Trip")
	}
	// The wrapped error must still be recognisable as a cancellation so callers
	// can distinguish "user aborted" from a real failure.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Err() = %v, want it to wrap context.Canceled", err)
	}

	select {
	case <-g.Done():
	case <-time.After(time.Second):
		t.Error("Done() not closed after Trip")
	}

	if ctxErr := g.Context().Err(); ctxErr == nil {
		t.Error("Context() not cancelled after Trip")
	}
}

// TestGuardTripKeepsFirstReason matters for diagnosis: the original cause is the
// useful one, and a later trip must not overwrite it.
func TestGuardTripKeepsFirstReason(t *testing.T) {
	g := newTestGuard(t, nil)
	defer g.Close()

	g.Trip("first")
	g.Trip("second")

	if reason := g.Reason(); reason != "first" {
		t.Errorf("Reason() = %q, want %q (first reason should win)", reason, "first")
	}
}

func TestGuardTripIsConcurrencySafe(t *testing.T) {
	g := newTestGuard(t, nil)
	defer g.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.Trip("concurrent")
			_ = g.Reason()
			_ = g.Tripped()
			_ = g.Err()
		}()
	}
	wg.Wait()

	if !g.Tripped() {
		t.Error("Guard not tripped after concurrent Trip calls")
	}
}

// TestGuardCustomConditionTrips exercises the watch loop without needing to move
// a real cursor.
func TestGuardCustomConditionTrips(t *testing.T) {
	var mu sync.Mutex
	shouldStop := false

	g := newTestGuard(t, &safety.Options{
		Custom: func() (bool, string) {
			mu.Lock()
			defer mu.Unlock()
			return shouldStop, "custom fired"
		},
	})
	defer g.Close()

	if g.Tripped() {
		t.Fatal("tripped before the custom condition returned true")
	}

	mu.Lock()
	shouldStop = true
	mu.Unlock()

	select {
	case <-g.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("custom condition did not trip the Guard")
	}

	if reason := g.Reason(); reason != "custom fired" {
		t.Errorf("Reason() = %q, want %q", reason, "custom fired")
	}
}

// TestGuardSleepIsInterrupted is the property that keeps an abort responsive:
// a plain time.Sleep between actions would ignore the stop request for its whole
// duration.
func TestGuardSleepIsInterrupted(t *testing.T) {
	g := newTestGuard(t, nil)
	defer g.Close()

	go func() {
		time.Sleep(30 * time.Millisecond)
		g.Trip("interrupt the sleep")
	}()

	start := time.Now()
	err := g.Sleep(10 * time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Sleep returned nil after being interrupted")
	}
	if elapsed > 2*time.Second {
		t.Errorf("Sleep took %v; it should have returned as soon as the Guard tripped", elapsed)
	}
}

func TestGuardSleepCompletesNormally(t *testing.T) {
	g := newTestGuard(t, nil)
	defer g.Close()

	start := time.Now()
	if err := g.Sleep(40 * time.Millisecond); err != nil {
		t.Errorf("Sleep returned %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Errorf("Sleep returned after %v, expected to wait ~40ms", elapsed)
	}
}

func TestGuardSleepRefusesWhenAlreadyTripped(t *testing.T) {
	g := newTestGuard(t, nil)
	defer g.Close()

	g.Trip("already stopped")

	start := time.Now()
	if err := g.Sleep(5 * time.Second); err == nil {
		t.Error("Sleep returned nil despite the Guard being tripped")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Sleep waited %v before refusing; it should return immediately", elapsed)
	}
}

// TestGuardClickRefusesWhenTripped covers the reason clicks are routed through
// the Guard at all: checking only at the top of a loop lets a queued action
// sequence keep clicking after the user asked it to stop.
func TestGuardClickRefusesWhenTripped(t *testing.T) {
	g := newTestGuard(t, nil)
	defer g.Close()

	g.Trip("stop before clicking")

	// Coordinates are irrelevant: the Guard must refuse before reaching the OS.
	if err := g.Click(10, 10, 0); err == nil {
		t.Error("Click returned nil despite the Guard being tripped")
	}
	if err := g.ClickWithOptions(10, 10, nil); err == nil {
		t.Error("ClickWithOptions returned nil despite the Guard being tripped")
	}
}

func TestGuardCloseIsIdempotentAndDoesNotTrip(t *testing.T) {
	g := newTestGuard(t, nil)

	if err := g.Close(); err != nil {
		t.Errorf("first Close() = %v, want nil", err)
	}
	for i := 0; i < 3; i++ {
		if err := g.Close(); err != nil {
			t.Errorf("repeat Close() = %v, want nil", err)
		}
	}

	// Close tears down the watcher; it is not itself an abort, so Reason stays
	// empty even though the context is finished.
	if reason := g.Reason(); reason != "" {
		t.Errorf("Reason() = %q after Close, want empty (Close is not an abort)", reason)
	}
}

func TestCornerString(t *testing.T) {
	cases := map[safety.Corner]string{
		safety.TopLeft:     "top-left",
		safety.TopRight:    "top-right",
		safety.BottomLeft:  "bottom-left",
		safety.BottomRight: "bottom-right",
		safety.Corner(99):  "unknown",
	}
	for corner, want := range cases {
		if got := corner.String(); got != want {
			t.Errorf("Corner(%d).String() = %q, want %q", corner, got, want)
		}
	}
}
