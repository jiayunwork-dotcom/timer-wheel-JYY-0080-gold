// Command timer-wheel is a small CLI that drives the hierarchical timer-wheel
// scheduler.
//
// It reads scheduling commands from standard input, one per line:
//
//	at <ms> <label>     schedule <label> to fire after <ms> milliseconds
//	cancel <id>        cancel the task previously assigned <id>
//
// When a task fires, its label is printed to standard output. The CLI uses the
// real wall clock, supports an optional auto-shutdown timeout, and shuts down
// cleanly on Ctrl-C (SIGINT/SIGTERM), cancelling any pending tasks.
//
// Example session (labels are printed as they fire):
//
//	$ printf 'at 100 hello\nat 50 world\n' | ./timer-wheel
//	world
//	hello
//
// Flags may appear before or after positionals; positionals are reordered
// ahead of flags so the standard flag package can parse them (its parser stops
// at the first non-flag argument).
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"timer-wheel/internal/clock"
	"timer-wheel/internal/scheduler"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "timer-wheel:", err)
		os.Exit(1)
	}
}

// run wires the CLI: it parses flags, builds the scheduler on the real clock,
// and pumps stdin commands until the context is cancelled.
func run(argv []string, in io.Reader, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("timer-wheel", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 0, "auto-shutdown after this duration (0 = run until interrupted)")
	fs.Usage = func() {
		fmt.Fprintln(errOut, "usage: timer-wheel [flags] [run]")
		fmt.Fprintln(errOut, "  reads 'at <ms> <label>' and 'cancel <id>' lines from stdin")
		fs.PrintDefaults()
	}
	if err := fs.Parse(reorderArgs(fs, argv)); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *timeout)
		defer cancel()
	}

	clk := clock.NewRealClock()
	s := scheduler.New(clk, scheduler.Options{})
	s.Start(ctx)

	readCommands(ctx, s, in, out, errOut)

	s.Shutdown()
	m := s.Metrics()
	fmt.Fprintf(errOut, "timer-wheel: scheduled=%d fired=%d cancelled=%d\n",
		m.Scheduled, m.Fired, m.Cancelled)
	return nil
}

// readCommands consumes stdin lines and translates them into scheduler calls.
func readCommands(ctx context.Context, s *scheduler.Scheduler, in io.Reader, out, errOut io.Writer) {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "at":
			handleAt(s, fields, errOut)
		case "cancel":
			handleCancel(s, fields, errOut)
		default:
			fmt.Fprintf(errOut, "timer-wheel: unknown command %q (want 'at' or 'cancel')\n", fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(errOut, "timer-wheel: read error: %v\n", err)
	}
}

// handleAt parses "at <ms> <label>" and schedules the label. The assigned id is
// reported on stderr so the operator can later cancel it.
func handleAt(s *scheduler.Scheduler, fields []string, errOut io.Writer) {
	if len(fields) < 3 {
		fmt.Fprintf(errOut, "timer-wheel: 'at' needs <ms> <label>\n")
		return
	}
	ms, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || ms < 0 {
		fmt.Fprintf(errOut, "timer-wheel: bad delay %q\n", fields[1])
		return
	}
	label := strings.Join(fields[2:], " ")
	id, err := s.Schedule(time.Duration(ms)*time.Millisecond, func() {
		fmt.Println(label)
	})
	if err != nil {
		fmt.Fprintf(errOut, "timer-wheel: schedule error: %v\n", err)
		return
	}
	fmt.Fprintf(errOut, "timer-wheel: scheduled id=%d label=%q\n", id, label)
}

// handleCancel parses "cancel <id>" and cancels the task.
func handleCancel(s *scheduler.Scheduler, fields []string, errOut io.Writer) {
	if len(fields) < 2 {
		fmt.Fprintf(errOut, "timer-wheel: 'cancel' needs <id>\n")
		return
	}
	id, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		fmt.Fprintf(errOut, "timer-wheel: bad id %q\n", fields[1])
		return
	}
	if !s.Cancel(id) {
		fmt.Fprintf(errOut, "timer-wheel: no live task with id=%d\n", id)
	}
}

// reorderArgs moves positional arguments ahead of flags so that the flag
// package (which stops parsing at the first non-flag) can handle invocations
// such as `timer-wheel run -timeout 5s`. Flag/value pairs supplied as two
// separate words are kept adjacent.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	var positional, flags []string
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" || a == "--" {
			positional = append(positional, a)
			i++
			continue
		}
		if strings.Contains(a, "=") {
			flags = append(flags, a)
			i++
			continue
		}
		// A bare flag: keep it together with a following non-flag value.
		flags = append(flags, a)
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			flags = append(flags, args[i+1])
			i += 2
			continue
		}
		i++
	}
	return append(positional, flags...)
}
