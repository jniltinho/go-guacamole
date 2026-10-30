// Package jobs runs GoGuacamole's background work on asynq (Redis) when configured: enqueueing, workers on queues
// prefixed "goguacamole:", a check that the Redis database belongs to this installation, and health for /ready and
// Administration. Without Redis, Enqueue reports ErrQueueUnavailable and periodic work stays in-process
// (openspec add-async-jobs-object-storage).
package jobs

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/jniltinho/go-guacamole/internal/config"
)

// Queues, by priority. Every queue of this application carries the prefix.
const (
	Prefix   = "goguacamole:"
	Critical = Prefix + "critical"
	Default  = Prefix + "default"
	Low      = Prefix + "low"
	markKey  = Prefix + "installation"
)

// ShutdownGrace is how long running jobs get to finish on shutdown before they are cancelled and returned to the
// queue (their leases are released, so another instance completes the work).
var ShutdownGrace = 20 * time.Second

// ErrQueueUnavailable is returned by Enqueue when no healthy queue exists.
var ErrQueueUnavailable = errors.New("job queue unavailable")

// State of the queue as shown in readiness details and Administration.
type State string

const (
	Disabled State = "disabled" // no Redis configured
	Up       State = "up"
	Down     State = "down"     // configured but unreachable (retrying)
	Conflict State = "conflict" // the Redis database holds another application's queues
)

// Health is a snapshot of the queue state.
type Health struct {
	State State  `json:"state"`
	Error string `json:"error,omitempty"`
}

// Handler processes one task payload. Return asynq.SkipRetry (wrapped) for failures that retrying cannot fix.
type Handler func(ctx context.Context, payload []byte) error

// Queue is the job queue of one instance.
type Queue struct {
	cfg          config.RedisConfig
	jobs         config.JobsConfig
	installation string
	log          *slog.Logger

	mux      *asynq.ServeMux
	handlers map[string]Handler
	periodic []Periodic
	leaser   Leaser
	owner    string
	failedAt map[string]time.Time // fallback backoff, only touched by the scheduler goroutine

	mu     sync.Mutex
	health Health
	client *asynq.Client
	server *asynq.Server
	rdb    redis.UniversalClient
}

// New prepares a queue; Start connects it. installation is the database's installation id.
func New(cfg config.RedisConfig, jobs config.JobsConfig, installation string, log *slog.Logger) *Queue {
	q := &Queue{cfg: cfg, jobs: jobs, installation: installation, log: log, mux: asynq.NewServeMux(), handlers: map[string]Handler{},
		health: Health{State: Disabled}, failedAt: map[string]time.Time{}}
	return q
}

// Enabled reports whether a Redis address is configured.
func (q *Queue) Enabled() bool { return q.cfg.Addr != "" }

// Register adds the handler of a task type (before Start).
func (q *Queue) Register(typ string, h Handler) {
	q.handlers[typ] = h
	q.mux.HandleFunc(typ, func(ctx context.Context, t *asynq.Task) error { return h(ctx, t.Payload()) })
}

// Handler returns the registered handler of a type, so the in-process fallback runs the same code.
func (q *Queue) Handler(typ string) Handler { return q.handlers[typ] }

func (q *Queue) redisOpt() asynq.RedisClientOpt {
	o := asynq.RedisClientOpt{Addr: q.cfg.Addr, Password: q.cfg.Password, DB: q.cfg.DB}
	if q.cfg.TLS {
		o.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return o
}

// Start connects to Redis, checks that the database is ours and starts the workers. When Redis is unreachable it
// keeps retrying in the background; sessions are never affected.
func (q *Queue) Start(ctx context.Context) {
	if !q.Enabled() {
		return
	}
	q.setHealth(Down, "connecting")
	go func() {
		for wait := time.Second; ; wait = min(wait*2, 30*time.Second) {
			err := q.connect(ctx)
			if err == nil || errors.Is(err, errConflict) {
				return
			}
			q.setHealth(Down, err.Error())
			q.log.Warn("job queue: redis unavailable, retrying", "err", err, "in", wait)
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
	}()
}

var errConflict = errors.New("redis database shared with another application")

func (q *Queue) connect(ctx context.Context) error {
	opt := q.redisOpt()
	rdb, ok := opt.MakeRedisClient().(redis.UniversalClient)
	if !ok {
		return errors.New("unexpected redis client")
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return err
	}
	if err := q.checkDedicated(ctx, rdb); err != nil {
		_ = rdb.Close()
		q.setHealth(Conflict, err.Error())
		q.log.Error("job queue disabled: the Redis database is not dedicated to this installation", "err", err)
		return errConflict
	}
	client := asynq.NewClient(opt)
	var server *asynq.Server
	if q.jobs.Worker {
		server = asynq.NewServer(opt, asynq.Config{
			Concurrency:     max(q.jobs.Concurrency, 1),
			Queues:          map[string]int{Critical: 6, Default: 3, Low: 1},
			ShutdownTimeout: ShutdownGrace,
			Logger:          slogAdapter{q.log},
			LogLevel:        asynq.WarnLevel,
		})
		if err := server.Start(q.mux); err != nil {
			_ = client.Close()
			_ = rdb.Close()
			return err
		}
	}
	q.mu.Lock()
	q.rdb, q.client, q.server = rdb, client, server
	q.health = Health{State: Up}
	q.mu.Unlock()
	q.log.Info("job queue up", "redis", q.cfg.Addr, "db", q.cfg.DB, "worker", q.jobs.Worker)
	return nil
}

// checkDedicated refuses a database holding queues without our prefix or the mark of another installation.
func (q *Queue) checkDedicated(ctx context.Context, rdb redis.UniversalClient) error {
	queues, err := rdb.SMembers(ctx, "asynq:queues").Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	for _, name := range queues {
		if !strings.HasPrefix(name, Prefix) {
			return fmt.Errorf("queue %q belongs to another application", name)
		}
	}
	if ok, err := rdb.SetNX(ctx, markKey, q.installation, 0).Result(); err != nil {
		return err
	} else if !ok {
		owner, err := rdb.Get(ctx, markKey).Result()
		if err != nil {
			return err
		}
		if owner != q.installation {
			return fmt.Errorf("database marked by installation %s", owner)
		}
	}
	return nil
}

// Enqueue schedules a task on a queue. opts are asynq options (TaskID, ProcessIn, MaxRetry, Timeout...).
func (q *Queue) Enqueue(ctx context.Context, queue, typ string, payload []byte, opts ...asynq.Option) error {
	q.mu.Lock()
	client, state := q.client, q.health.State
	q.mu.Unlock()
	if client == nil || state != Up {
		return ErrQueueUnavailable
	}
	_, err := client.EnqueueContext(ctx, asynq.NewTask(typ, payload), append([]asynq.Option{asynq.Queue(queue)}, opts...)...)
	return err
}

// Health returns the current state.
func (q *Queue) Health() Health {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.health
}

// RedisOpt is the connection used by the dashboard.
func (q *Queue) RedisOpt() asynq.RedisClientOpt { return q.redisOpt() }

func (q *Queue) setHealth(s State, e string) {
	q.mu.Lock()
	q.health = Health{State: s, Error: e}
	q.mu.Unlock()
}

// Shutdown stops taking tasks and waits for running ones (asynq returns unfinished tasks to the queue).
func (q *Queue) Shutdown() {
	q.mu.Lock()
	server, client, rdb := q.server, q.client, q.rdb
	q.mu.Unlock()
	if server != nil {
		server.Shutdown()
	}
	if client != nil {
		_ = client.Close()
	}
	if rdb != nil {
		_ = rdb.Close()
	}
}

// slogAdapter routes asynq's logs to the application logger.
type slogAdapter struct{ l *slog.Logger }

func (s slogAdapter) Debug(args ...any) { s.l.Debug(fmt.Sprint(args...)) }
func (s slogAdapter) Info(args ...any)  { s.l.Info(fmt.Sprint(args...)) }
func (s slogAdapter) Warn(args ...any)  { s.l.Warn(fmt.Sprint(args...)) }
func (s slogAdapter) Error(args ...any) { s.l.Error(fmt.Sprint(args...)) }
func (s slogAdapter) Fatal(args ...any) { s.l.Error(fmt.Sprint(args...)) }
