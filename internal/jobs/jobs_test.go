package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jniltinho/go-guacamole/internal/config"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestWithoutRedis(t *testing.T) {
	q := New(config.RedisConfig{}, config.JobsConfig{Worker: true, Concurrency: 1}, "inst", quiet)
	q.Start(context.Background())
	if q.Enabled() || q.Health().State != Disabled {
		t.Fatalf("health: %+v", q.Health())
	}
	if err := q.Enqueue(context.Background(), Default, "x", nil); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("enqueue without redis: %v", err)
	}
}

// redisDB returns a clean Redis database for one test (GG_TEST_REDIS=host:port), or skips.
func redisDB(t *testing.T, db int) config.RedisConfig {
	addr := os.Getenv("GG_TEST_REDIS")
	if addr == "" {
		t.Skip("set GG_TEST_REDIS=host:port to run queue tests against Redis")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: db})
	if err := rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.FlushDB(context.Background()).Err(); _ = rdb.Close() })
	return config.RedisConfig{Addr: addr, DB: db}
}

func waitState(t *testing.T, q *Queue, want State) {
	t.Helper()
	for i := 0; i < 100 && q.Health().State != want; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if q.Health().State != want {
		t.Fatalf("state %+v, want %s", q.Health(), want)
	}
}

func TestEnqueueAndProcess(t *testing.T) {
	rc := redisDB(t, 1)
	q := New(rc, config.JobsConfig{Worker: true, Concurrency: 1}, "inst-a", quiet)
	got := make(chan string, 1)
	q.Register("test:echo", func(ctx context.Context, p []byte) error { got <- string(p); return nil })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)
	defer q.Shutdown()
	waitState(t, q, Up)
	if err := q.Enqueue(ctx, Default, "test:echo", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	select {
	case p := <-got:
		if p != "hello" {
			t.Fatalf("payload %q", p)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("task not processed")
	}
}

func TestSharedDatabaseRefused(t *testing.T) {
	rc := redisDB(t, 2)
	rdb := redis.NewClient(&redis.Options{Addr: rc.Addr, DB: rc.DB})
	defer rdb.Close()
	rdb.SAdd(context.Background(), "asynq:queues", "default") // another application's asynq queue
	q := New(rc, config.JobsConfig{Worker: true, Concurrency: 1}, "inst-a", quiet)
	q.Start(context.Background())
	defer q.Shutdown()
	waitState(t, q, Conflict)
	if err := q.Enqueue(context.Background(), Default, "x", nil); !errors.Is(err, ErrQueueUnavailable) {
		t.Fatalf("enqueue on a shared database: %v", err)
	}
}

func TestOtherInstallationRefused(t *testing.T) {
	rc := redisDB(t, 3)
	a := New(rc, config.JobsConfig{Worker: false, Concurrency: 1}, "inst-a", quiet)
	a.Start(context.Background())
	defer a.Shutdown()
	waitState(t, a, Up)
	b := New(rc, config.JobsConfig{Worker: false, Concurrency: 1}, "inst-b", quiet)
	b.Start(context.Background())
	defer b.Shutdown()
	waitState(t, b, Conflict)
}
