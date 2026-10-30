package jobs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/store/storetest"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func newStore(t *testing.T) *store.Store {
	return storetest.Open(t, "sqlite", filepath.Join(t.TempDir(), "jobs.db"))
}

func daily(n *atomic.Int32) jobs.Periodic {
	return jobs.Periodic{Name: "test:daily", Queue: jobs.Critical, Every: 24 * time.Hour, Lease: time.Minute,
		Run: func(ctx context.Context, _ string) error { n.Add(1); return nil }}
}

func TestPeriodRunsOnce(t *testing.T) {
	st, ctx := newStore(t), context.Background()
	var n atomic.Int32
	p := daily(&n)
	period := jobs.Period(time.Now(), p.Every)
	for i := 0; i < 3; i++ { // first run, a retry and a re-enqueue after the queue forgot the TaskID
		if err := jobs.RunPeriod(ctx, st, "inst-a", p, period); err != nil {
			t.Fatal(err)
		}
	}
	if n.Load() != 1 {
		t.Fatalf("runs for one period: %d", n.Load())
	}
}

func TestManualRunRefusedWhileScheduledRuns(t *testing.T) {
	st, ctx := newStore(t), context.Background()
	started, release := make(chan struct{}), make(chan struct{})
	p := jobs.Periodic{Name: "recording:purge", Every: 24 * time.Hour, Lease: time.Minute,
		Run: func(ctx context.Context, _ string) error { close(started); <-release; return nil }}
	done := make(chan error, 1)
	go func() { done <- jobs.RunPeriod(ctx, st, "inst-a", p, jobs.Period(time.Now(), p.Every)) }()
	<-started
	if err := jobs.RunNow(ctx, st, "inst-b", p); !errors.Is(err, jobs.ErrBusy) {
		t.Fatalf("manual run during the scheduled one: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	p.Run = func(context.Context, string) error { return nil }
	if err := jobs.RunNow(ctx, st, "inst-b", p); err != nil {
		t.Fatalf("manual run after it finished: %v", err)
	}
}

// A paused run whose lease expired and was taken over cannot record its period nor free the new lease.
func TestStaleRunCannotCompleteOrRelease(t *testing.T) {
	st, ctx := newStore(t), context.Background()
	now := time.Now().UnixMilli()
	if ok, err := st.AcquireLease(ctx, "j", "inst-a", "tok-a", now, now+10); !ok || err != nil {
		t.Fatalf("first lease: %v %v", ok, err)
	}
	later := now + 1000
	if ok, err := st.AcquireLease(ctx, "j", "inst-b", "tok-b", later, later+60_000); !ok || err != nil {
		t.Fatalf("takeover of the expired lease: %v %v", ok, err)
	}
	if ok, err := st.CompletePeriod(ctx, "j", "2026-10-30", "tok-a", later); ok || err != nil {
		t.Fatalf("stale run completed its period: %v %v", ok, err)
	}
	if ok, _ := st.RenewLease(ctx, "j", "tok-a", later, later+60_000); ok {
		t.Fatal("stale run renewed")
	}
	_ = st.ReleaseLease(ctx, "j", "tok-a")
	if ok, _ := st.AcquireLease(ctx, "j", "inst-c", "tok-c", later, later+60_000); ok {
		t.Fatal("the stale run freed the live lease")
	}
	if done, _ := st.PeriodDone(ctx, "j", "2026-10-30"); done {
		t.Fatal("period recorded by a stale run")
	}
}

// Without Redis the scheduler runs due periods in-process, still once (a missed period runs at the first tick).
func TestFallbackTickRunsOncePerPeriod(t *testing.T) {
	st, ctx := newStore(t), context.Background()
	var n atomic.Int32
	q := jobs.New(config.RedisConfig{}, config.JobsConfig{Worker: true, Concurrency: 1}, "inst", quiet)
	q.Schedule(st, "inst-a", []jobs.Periodic{daily(&n)})
	now := time.Now()
	q.Tick(ctx, now)
	q.Tick(ctx, now.Add(30*time.Second))
	if n.Load() != 1 {
		t.Fatalf("fallback runs: %d", n.Load())
	}
}

// Three replicas on one Redis enqueue the same period; it runs once.
func TestThreeReplicasOneRun(t *testing.T) {
	addr := os.Getenv("GG_TEST_REDIS")
	if addr == "" {
		t.Skip("set GG_TEST_REDIS=host:port")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 4})
	defer rdb.Close()
	_ = rdb.FlushDB(context.Background()).Err()
	st := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var n atomic.Int32
	var qs []*jobs.Queue
	for i := 0; i < 3; i++ {
		q := jobs.New(config.RedisConfig{Addr: addr, DB: 4}, config.JobsConfig{Worker: true, Concurrency: 2}, "inst", quiet)
		q.Schedule(st, "replica-"+string(rune('a'+i)), []jobs.Periodic{daily(&n)})
		q.Start(ctx)
		qs = append(qs, q)
	}
	for _, q := range qs {
		for i := 0; i < 100 && q.Health().State != jobs.Up; i++ {
			time.Sleep(50 * time.Millisecond)
		}
		defer q.Shutdown()
	}
	now := time.Now()
	for _, q := range qs {
		q.Tick(ctx, now)
	}
	time.Sleep(3 * time.Second)
	if n.Load() != 1 {
		t.Fatalf("runs across replicas: %d", n.Load())
	}
}

// An instance stopped during a job returns it to the queue; another instance completes the period exactly once.
func TestShutdownDuringJob(t *testing.T) {
	addr := os.Getenv("GG_TEST_REDIS")
	if addr == "" {
		t.Skip("set GG_TEST_REDIS=host:port")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 5})
	defer rdb.Close()
	_ = rdb.FlushDB(context.Background()).Err()
	old := jobs.ShutdownGrace
	jobs.ShutdownGrace = 500 * time.Millisecond
	defer func() { jobs.ShutdownGrace = old }()
	st := newStore(t)
	var started, completed atomic.Int32
	p := jobs.Periodic{Name: "test:long", Queue: jobs.Default, Every: 24 * time.Hour, Lease: time.Minute,
		Run: func(ctx context.Context, _ string) error {
			if started.Add(1) == 1 { // the first attempt is interrupted by the shutdown
				<-ctx.Done()
				return ctx.Err()
			}
			completed.Add(1)
			return nil
		}}
	rc := config.RedisConfig{Addr: addr, DB: 5}
	jc := config.JobsConfig{Worker: true, Concurrency: 1}
	ctx := context.Background()
	a := jobs.New(rc, jc, "inst", quiet)
	a.Schedule(st, "replica-a", []jobs.Periodic{p})
	a.Start(ctx)
	for i := 0; i < 100 && a.Health().State != jobs.Up; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	a.Tick(ctx, time.Now())
	for i := 0; i < 100 && started.Load() == 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	a.Shutdown()
	b := jobs.New(rc, jc, "inst", quiet)
	b.Schedule(st, "replica-b", []jobs.Periodic{p})
	b.Start(ctx)
	defer b.Shutdown()
	for i := 0; i < 400 && completed.Load() == 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if completed.Load() != 1 {
		t.Fatalf("completed %d times (started %d)", completed.Load(), started.Load())
	}
	if done, _ := st.PeriodDone(ctx, p.Name, jobs.Period(time.Now(), p.Every)); !done {
		t.Fatal("period not recorded")
	}
}
