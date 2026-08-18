# timer-wheel

`timer-wheel` is a small, dependency-free Go library that implements a
**hierarchical timing-wheel scheduler** — a data structure used for efficiently
managing large numbers of timers with fine-grained, out-of-order cancellation.
It is the same family of algorithm that powers job schedulers, network
timeouts, and rate limiters. The library is driven by an injectable `Clock`, so
its behaviour is fully deterministic and unit-testable without ever sleeping on
the real wall clock.

The package is organised into three internal layers:

- `internal/clock` — a `Clock` abstraction plus a `FakeClock` for tests. The fake
  clock only moves when you call `Advance`, firing every due timer
  synchronously and in chronological order.
- `internal/wheel` — the hierarchical timing wheel itself: multiple levels of
  buckets, overflow cascading, `Add` / `Cancel` / `Advance`, and guaranteed
  in-order firing.
- `internal/scheduler` — a user-facing facade (`Schedule`, `Cancel`,
  `Shutdown`) with a clock-driven run loop, context-cancellation propagation,
  and live metrics (scheduled / fired / cancelled counts).

A runnable CLI (`main.go`) reads scheduling commands from standard input and
fires labels on the real clock, shutting down cleanly on Ctrl-C.

## Building

```sh
go build ./...
go test ./...
go vet ./...
```

The project targets Go 1.21 and uses only the standard library.

## Using the library

```go
import (
    "context"
    "time"

    "timer-wheel/internal/clock"
    "timer-wheel/internal/scheduler"
)

func main() {
    clk := clock.NewRealClock()
    s := scheduler.New(clk, scheduler.Options{})
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    s.Start(ctx)
    id, _ := s.Schedule(2*time.Second, func() { println("done") })
    _ = s.Cancel(id) // remove it before it fires
    s.Shutdown()     // cancel pending work and observe cancellation
}
```

For deterministic tests, swap the real clock for `clock.NewFakeClock` and call
`Advance` to drive time forward:

```go
fc := clock.NewFakeClock(time.Unix(0, 0))
s := scheduler.New(fc, scheduler.Options{})
s.Start(context.Background())
s.Schedule(100*time.Millisecond, func() { /* fires when the clock advances */ })
fc.Advance(100 * time.Millisecond)
```

## CLI

```sh
# schedule two labels; they print as they fire (earliest first)
printf 'at 100 hello\nat 50 world\n' | ./timer-wheel
```

Input grammar (one command per line):

- `at <ms> <label>` — schedule `label` to fire after `<ms>` milliseconds. The
  assigned task id is reported on stderr so it can be cancelled.
- `cancel <id>` — cancel a previously scheduled task before it fires.

Flags such as `-timeout 5s` may appear before or after the command; the CLI
reorders positionals ahead of flags so the standard `flag` package can parse
them.

## Design notes

- **Determinism.** Every time-dependent component takes a `Clock`. Tests use a
  `FakeClock`, so firing order and cancellation are reproducible and
  flake-free.
- **Hierarchy.** The wheel is composed of levels; each level is a ring of
  buckets. Tasks whose delay exceeds a level's span overflow into the next
  level and cascade back down as the clock advances.
- **Cancellation.** `Cancel` removes a task before it fires; `Shutdown` cancels
  all pending tasks and cancels the scheduler's base context, so any dependent
  work observes cancellation.
- **Metrics.** Cumulative `Scheduled`, `Fired`, and `Cancelled` counters are
  always available via `Metrics()`.

## License

Released under the MIT License. See [LICENSE](./LICENSE).
