package wheel

import (
	"sync"
	"time"
)

// wheelEpoch is the fixed reference used to compute bucket slots. Bucket
// boundaries are floor(expiration / tick) * tick relative to this epoch, which
// keeps them stable as the clock advances.
var wheelEpoch = time.Unix(0, 0).UTC()

// TaskFunc is the callback invoked when a scheduled entry fires.
type TaskFunc func()

// entry is a single scheduled unit of work inside the wheel. It lives in
// exactly one place at a time: a bucket (waiting), the expiry queue (due
// within the current tick), or nowhere (fired or cancelled).
type entry struct {
	ID         int64
	expiration time.Time
	fn         TaskFunc
	priority   int
	cancelled  bool
	bucket     *bucket
}

// bucket is one slot of a wheel level. All entries hashed into the same bucket
// share the same firing window; the entry heap (queue) guarantees they fire in
// true chronological order when the bucket is flushed.
type bucket struct {
	expiration time.Time
	entries    []*entry
}

// removeEntry deletes e from b (if present) and clears its back-reference.
func removeEntry(b *bucket, e *entry) {
	for i, cur := range b.entries {
		if cur == e {
			b.entries = append(b.entries[:i], b.entries[i+1:]...)
			e.bucket = nil
			return
		}
	}
}

// Wheel is a hierarchical timing wheel. Time is organised into levels; each
// level is a ring of wheelSize buckets. Tasks whose delay is too large for a
// level overflow into the next level. On Advance the clock is moved forward and
// due buckets are cascaded down until every task whose deadline has passed is
// fired in chronological order.
//
// The zero value is not usable; construct with New.
type Wheel struct {
	mu         sync.Mutex
	tick       time.Duration
	wheelSize  int
	interval   time.Duration // tick * wheelSize
	currentTime time.Time
	buckets    []*bucket
	overflow   *Wheel
	root       *Wheel
	ready      *bucketHeap
	queue      *entryHeap
	entries    map[int64]*entry
	seq        int64
}

// Options configures a Wheel created by New.
type Options struct {
	// Tick is the finest time granularity of the top level. Must be > 0.
	Tick time.Duration
	// WheelSize is the number of buckets per level. Must be > 0.
	WheelSize int
	// Start is the anchor time the wheel treats as "now" before any Advance.
	// Defaults to the Unix epoch when zero.
	Start time.Time
}

func (o Options) normalize() (time.Duration, int, time.Time) {
	tick := o.Tick
	if tick <= 0 {
		tick = time.Millisecond
	}
	ws := o.WheelSize
	if ws <= 0 {
		ws = 20
	}
	start := o.Start
	if start.IsZero() {
		start = time.Unix(0, 0).UTC()
	}
	return tick, ws, start
}

// New constructs a hierarchical timing wheel.
func New(opts Options) *Wheel {
	tick, ws, start := opts.normalize()
	w := newLevel(tick, ws, start)
	w.root = w
	w.entries = make(map[int64]*entry)
	w.ready = &bucketHeap{}
	w.queue = &entryHeap{}
	heapInit(w.ready)
	heapInit(w.queue)
	return w
}

func newLevel(tick time.Duration, ws int, start time.Time) *Wheel {
	w := &Wheel{
		tick:       tick,
		wheelSize:  ws,
		interval:   tick * time.Duration(ws),
		currentTime: start,
		buckets:    make([]*bucket, ws),
	}
	for i := range w.buckets {
		w.buckets[i] = &bucket{}
	}
	return w
}

// Now returns the wheel's current notion of time.
func (w *Wheel) Now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.currentTime
}

// SetNow anchors the wheel's current time. It must only be called before any
// task is scheduled; it lets the real-clock run loop align the wheel to the
// actual wall clock so delays are interpreted relative to "now" rather than the
// Unix epoch.
func (w *Wheel) SetNow(t time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.currentTime = t
}

// Add schedules fn to run delay after the current wheel time, under the given
// id. A non-positive delay schedules the task to fire on the next Advance.
// If id is already scheduled it is replaced. Tasks that become due at the same
// instant are fired in descending priority order; ties are broken by ascending
// id so behaviour is fully deterministic.
func (w *Wheel) Add(id int64, delay time.Duration, priority int, fn TaskFunc) {
	if delay < 0 {
		delay = 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if old, ok := w.entries[id]; ok {
		old.cancelled = true
		if old.bucket != nil {
			removeEntry(old.bucket, old)
		}
		delete(w.entries, id)
	}
	w.seq++
	e := &entry{ID: id, expiration: w.currentTime.Add(delay), fn: fn, priority: priority}
	w.entries[id] = e
	w.root.addEntry(e)
}

// Cancel removes a scheduled task before it fires. It reports whether a live
// task with that id was actually cancelled.
func (w *Wheel) Cancel(id int64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	e, ok := w.entries[id]
	if !ok {
		return false
	}
	delete(w.entries, id)
	e.cancelled = true
	if e.bucket != nil {
		removeEntry(e.bucket, e)
		e.bucket = nil
	}
	return true
}

// PendingIDs returns the ids of all currently scheduled (not yet fired,
// not cancelled) tasks.
func (w *Wheel) PendingIDs() []int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	ids := make([]int64, 0, len(w.entries))
	for id := range w.entries {
		ids = append(ids, id)
	}
	return ids
}

// Len returns the number of currently scheduled tasks.
func (w *Wheel) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.entries)
}

// NextDeadline returns the earliest expiration among scheduled tasks, or the
// zero time if none are pending. The real-clock run loop uses this to decide
// how long to sleep before the next Advance.
func (w *Wheel) NextDeadline() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	var min time.Time
	for _, e := range w.entries {
		if e.cancelled {
			continue
		}
		if min.IsZero() || e.expiration.Before(min) {
			min = e.expiration
		}
	}
	return min
}

// AdvanceBy moves the clock forward by d and fires everything that becomes due.
func (w *Wheel) AdvanceBy(d time.Duration) {
	w.mu.Lock()
	target := w.currentTime.Add(d)
	w.mu.Unlock()
	w.Advance(target)
}

// Advance moves the wheel's clock to now (ignored if now is in the past) and
// fires all tasks whose deadline is at or before now, in chronological order.
// Callbacks are invoked after the internal state is updated, so a callback may
// safely schedule new tasks.
func (w *Wheel) Advance(now time.Time) {
	w.mu.Lock()
	if now.Before(w.currentTime) {
		w.mu.Unlock()
		return
	}
	w.advanceClock(now)
	w.flush(now)

	var fired []*entry
	for {
		e := w.queue.peek()
		if e == nil || e.expiration.After(now) {
			break
		}
		heapPop(w.queue)
		if e.cancelled {
			continue
		}
		fired = append(fired, e)
	}
	for _, e := range fired {
		delete(w.entries, e.ID)
	}
	w.mu.Unlock()

	for _, e := range fired {
		if e.fn != nil {
			e.fn()
		}
	}
}

// advanceClock aligns the level's current time up to now and recurses into the
// overflow level so every level stays in lock-step.
func (w *Wheel) advanceClock(now time.Time) {
	if now.Before(w.currentTime) {
		return
	}
	if now.Sub(w.currentTime) < w.tick {
		return
	}
	steps := now.Sub(w.currentTime) / w.tick
	w.currentTime = w.currentTime.Add(steps * w.tick)
	if w.overflow != nil {
		w.overflow.advanceClock(w.currentTime)
	}
}

// flush re-inserts every bucket whose expiration is at or before now. Re-insert
// routes tasks either to the expiry queue (if they are now within a tick of the
// advanced clock) or back into a lower/higher bucket, cascading them toward
// their firing instant.
func (w *Wheel) flush(now time.Time) {
	for {
		b := w.ready.peek()
		if b == nil || b.expiration.After(now) {
			break
		}
		heapPop(w.ready)
		entries := b.entries
		b.entries = nil
		b.expiration = time.Time{}
		for _, e := range entries {
			if e.cancelled {
				continue
			}
			e.bucket = nil
			w.root.addEntry(e)
		}
	}
}

// addEntry places an entry into the wheel. It is the recursive core used by
// both Add and the cascade (flush). The expiry queue and ready heap live on the
// root level so all levels share a single ordering.
func (w *Wheel) addEntry(e *entry) {
	if e.cancelled {
		return
	}
	if e.expiration.Before(w.currentTime.Add(w.tick)) {
		heapPush(w.root.queue, e)
		return
	}
	if e.expiration.Before(w.currentTime.Add(w.interval)) {
		// Use the absolute virtual id (relative to a fixed epoch) so the bucket
		// slot and its expiration are stable across clock advances. This is what
		// makes the cascade terminate instead of re-flushing the same bucket.
		v := int64(e.expiration.Sub(wheelEpoch) / w.tick)
		rem := v % int64(w.wheelSize)
		idx := int(rem)
		b := w.buckets[idx]
		if b.expiration.IsZero() {
			b.expiration = wheelEpoch.Add(time.Duration(v) * w.tick)
			heapPush(w.root.ready, b)
		}
		b.entries = append(b.entries, e)
		e.bucket = b
		return
	}
	if w.overflow == nil {
		w.overflow = newLevel(w.interval, w.wheelSize, w.currentTime)
		w.overflow.root = w.root
		w.overflow.ready = w.root.ready
		w.overflow.queue = w.root.queue
	}
	w.overflow.addEntry(e)
}
