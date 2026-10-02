package clock

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Mock is a fake Clock for tests. Its time stands still until the test calls
// Advance or Set; then every timer, ticker and AfterFunc whose deadline has
// been reached fires, in deadline order, with Now reporting that deadline.
//
// Sleep, SleepContext and After block until another goroutine advances the
// clock far enough. Use BlockUntil to wait until the code under test has
// registered its timers before advancing, which removes sleeps and flakiness
// from tests.
//
// The zero value is not usable; create a Mock with NewMock. A Mock is safe
// for concurrent use.
type Mock struct {
	mu      sync.Mutex
	now     time.Time
	seq     uint64
	waiters []*mockWaiter
	changed chan struct{} // closed and replaced whenever len(waiters) changes
}

// NewMock returns a Mock whose current time is start.
func NewMock(start time.Time) *Mock {
	return &Mock{now: start, changed: make(chan struct{})}
}

// mockWaiter is a pending timer, ticker or AfterFunc.
type mockWaiter struct {
	mock     *Mock
	deadline time.Time
	seq      uint64        // creation order, breaks deadline ties
	period   time.Duration // tick interval, guarded by mock.mu
	ticker   bool          // immutable after creation
	ch       chan time.Time
	fn       func()
	active   bool
}

var _ Clock = (*Mock)(nil)

// Now returns the mock's current time.
func (m *Mock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Since returns m.Now().Sub(t).
func (m *Mock) Since(t time.Time) time.Duration { return m.Now().Sub(t) }

// Until returns t.Sub(m.Now()).
func (m *Mock) Until(t time.Time) time.Duration { return t.Sub(m.Now()) }

// Sleep blocks until the clock has been advanced by at least d.
// A non-positive d returns immediately.
func (m *Mock) Sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	<-m.NewTimer(d).C()
}

// SleepContext blocks until the clock has been advanced by d or ctx is done.
func (m *Mock) SleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := m.NewTimer(d)
	return sleepContext(ctx, t.C(), t.Stop)
}

// After returns a channel that receives the mock time once the clock has
// been advanced by d.
func (m *Mock) After(d time.Duration) <-chan time.Time { return m.NewTimer(d).C() }

// NewTimer returns a Timer that fires once the clock has been advanced by d.
// A non-positive d fires immediately, as with time.NewTimer.
func (m *Mock) NewTimer(d time.Duration) Timer {
	w := &mockWaiter{mock: m, ch: make(chan time.Time, 1)}
	m.schedule(w, d)
	return w
}

// AfterFunc calls f once the clock has been advanced by d. The call happens
// synchronously inside Advance or Set, so the test observes its effects as
// soon as Advance returns. A non-positive d runs f in a new goroutine
// straight away, as time.AfterFunc does.
func (m *Mock) AfterFunc(d time.Duration, f func()) Timer {
	w := &mockWaiter{mock: m, fn: f}
	m.schedule(w, d)
	return w
}

// NewTicker returns a Ticker that ticks every d of mock time.
// It panics if d <= 0, as time.NewTicker does.
func (m *Mock) NewTicker(d time.Duration) Ticker {
	if d <= 0 {
		panic("clock: non-positive interval for NewTicker")
	}
	w := &mockWaiter{mock: m, ch: make(chan time.Time, 1), period: d, ticker: true}
	m.schedule(w, d)
	return mockTicker{w}
}

// Advance moves the clock forward by d, firing everything that becomes due.
// AfterFunc callbacks run synchronously, one at a time and in deadline
// order, before Advance returns; a negative d is ignored.
func (m *Mock) Advance(d time.Duration) {
	if d < 0 {
		return
	}
	m.mu.Lock()
	target := m.now.Add(d)
	m.mu.Unlock()
	m.advanceTo(target)
}

// Set moves the clock to t. Moving forward fires due timers exactly like
// Advance; moving backward only changes Now and fires nothing.
func (m *Mock) Set(t time.Time) {
	m.mu.Lock()
	if !t.After(m.now) {
		m.now = t
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	m.advanceTo(t)
}

// Waiters returns the number of active timers, tickers and AfterFuncs,
// including goroutines blocked in Sleep or After.
func (m *Mock) Waiters() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.waiters)
}

// BlockUntil blocks until at least n timers, tickers or sleepers are
// registered on the clock. Call it before Advance to make sure the code
// under test has reached its wait.
func (m *Mock) BlockUntil(n int) {
	_ = m.BlockUntilContext(context.Background(), n)
}

// BlockUntilContext is like BlockUntil but gives up when ctx is done,
// returning ctx.Err().
func (m *Mock) BlockUntilContext(ctx context.Context, n int) error {
	for {
		m.mu.Lock()
		if len(m.waiters) >= n {
			m.mu.Unlock()
			return nil
		}
		changed := m.changed
		m.mu.Unlock()

		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (m *Mock) advanceTo(target time.Time) {
	for {
		m.mu.Lock()
		w := m.nextDueLocked(target)
		if w == nil {
			if target.After(m.now) {
				m.now = target
			}
			m.mu.Unlock()
			return
		}
		if w.deadline.After(m.now) {
			m.now = w.deadline
		}
		now := m.now
		fn := w.fireLocked(now)
		m.mu.Unlock()

		if fn != nil {
			fn()
		}
	}
}

// nextDueLocked returns the earliest waiter due at or before target.
func (m *Mock) nextDueLocked(target time.Time) *mockWaiter {
	if len(m.waiters) == 0 || m.waiters[0].deadline.After(target) {
		return nil
	}
	return m.waiters[0]
}

// schedule (re)arms w to fire d after the current mock time.
func (m *Mock) schedule(w *mockWaiter, d time.Duration) (wasActive bool) {
	m.mu.Lock()
	wasActive = w.active
	if wasActive {
		m.removeLocked(w)
	}
	drain(w.ch)
	m.seq++
	w.seq = m.seq
	w.deadline = m.now.Add(d)
	if d <= 0 {
		// Due immediately, like time.NewTimer(0).
		fn := w.fireLocked(m.now)
		m.mu.Unlock()
		if fn != nil {
			go fn()
		}
		return wasActive
	}
	m.insertLocked(w)
	m.mu.Unlock()
	return wasActive
}

func (m *Mock) insertLocked(w *mockWaiter) {
	i := sort.Search(len(m.waiters), func(i int) bool {
		o := m.waiters[i]
		return o.deadline.After(w.deadline) || (o.deadline.Equal(w.deadline) && o.seq > w.seq)
	})
	m.waiters = append(m.waiters, nil)
	copy(m.waiters[i+1:], m.waiters[i:])
	m.waiters[i] = w
	w.active = true
	m.notifyLocked()
}

func (m *Mock) removeLocked(w *mockWaiter) {
	for i, o := range m.waiters {
		if o == w {
			m.waiters = append(m.waiters[:i], m.waiters[i+1:]...)
			break
		}
	}
	w.active = false
	m.notifyLocked()
}

func (m *Mock) notifyLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

// fireLocked delivers a tick or timer expiry at now and returns the
// AfterFunc callback to run, if any, once the lock is released.
func (w *mockWaiter) fireLocked(now time.Time) func() {
	m := w.mock
	if w.active {
		m.removeLocked(w)
	}
	if w.ch != nil {
		// Like time.Ticker, a slow receiver misses ticks instead of
		// blocking the clock.
		select {
		case w.ch <- now:
		default:
		}
	}
	if w.ticker {
		w.deadline = w.deadline.Add(w.period)
		if !w.deadline.After(now) {
			w.deadline = now.Add(w.period)
		}
		m.seq++
		w.seq = m.seq
		m.insertLocked(w)
	}
	return w.fn
}

func drain(ch chan time.Time) {
	if ch == nil {
		return
	}
	select {
	case <-ch:
	default:
	}
}

// C returns the channel on which the time is delivered.
func (w *mockWaiter) C() <-chan time.Time { return w.ch }

// Stop deactivates the timer or ticker and reports whether it was active.
// No value is left in the channel after Stop.
func (w *mockWaiter) Stop() bool {
	m := w.mock
	m.mu.Lock()
	defer m.mu.Unlock()
	wasActive := w.active
	if wasActive {
		m.removeLocked(w)
	}
	drain(w.ch)
	return wasActive
}

// Reset re-arms the timer d after the current mock time (for a ticker it
// also changes the period) and reports whether it had been active.
func (w *mockWaiter) Reset(d time.Duration) bool {
	if w.ticker {
		if d <= 0 {
			panic("clock: non-positive interval for Ticker.Reset")
		}
		w.mock.mu.Lock()
		w.period = d
		w.mock.mu.Unlock()
	}
	return w.mock.schedule(w, d)
}

// mockTicker adapts mockWaiter to the Ticker interface, whose Reset has no
// result.
type mockTicker struct{ *mockWaiter }

func (t mockTicker) Reset(d time.Duration) { t.mockWaiter.Reset(d) }
func (t mockTicker) Stop()                 { t.mockWaiter.Stop() }
