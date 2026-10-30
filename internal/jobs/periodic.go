package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

// Periodic is upkeep that runs once per UTC period (Every: a minute, five minutes, a day...).
type Periodic struct {
	Name  string
	Queue string
	Every time.Duration
	Lease time.Duration // lease TTL, renewed every Lease/3 while running
	Run   func(ctx context.Context, period string) error
}

// Leaser is the database side: leases and completed periods (internal/store).
type Leaser interface {
	PeriodDone(ctx context.Context, job, period string) (bool, error)
	AcquireLease(ctx context.Context, job, owner, token string, now, until int64) (bool, error)
	RenewLease(ctx context.Context, job, token string, now, until int64) (bool, error)
	CompletePeriod(ctx context.Context, job, period, token string, now int64) (bool, error)
	ReleaseLease(ctx context.Context, job, token string) error
}

var (
	// ErrBusy: another run of the job holds its lease.
	ErrBusy = errors.New("job already running")
	// ErrLeaseLost: the run lost its lease (expired, taken over) and its result was not recorded.
	ErrLeaseLost = errors.New("job lease lost")
)

// Period labels the UTC period containing t: "2006-01-02" for daily jobs, "2006-01-02T15:04" otherwise.
func Period(t time.Time, every time.Duration) string {
	t = t.UTC().Truncate(every)
	if every >= 24*time.Hour {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02T15:04")
}

func token() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func nowMs() int64 { return time.Now().UnixMilli() }

type periodKey struct{}

// PeriodOf returns the period a job run is working on ("" outside periodic jobs, "manual" for RunNow).
func PeriodOf(ctx context.Context) string {
	p, _ := ctx.Value(periodKey{}).(string)
	return p
}

// withLease runs fn while holding the job's lease; ctx is cancelled when a renewal fails.
func withLease(ctx context.Context, l Leaser, owner string, p Periodic, before func(context.Context) (bool, error),
	fn func(context.Context) error, after func(context.Context, string) (bool, error)) error {
	tok := token()
	ok, err := l.AcquireLease(ctx, p.Name, owner, tok, nowMs(), nowMs()+p.Lease.Milliseconds())
	if err != nil {
		return err
	}
	if !ok {
		return ErrBusy
	}
	if before != nil {
		if skip, err := before(ctx); err != nil || skip {
			_ = l.ReleaseLease(context.WithoutCancel(ctx), p.Name, tok)
			return err
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		t := time.NewTicker(max(p.Lease/3, time.Second))
		defer t.Stop()
		for {
			select {
			case <-stopped:
				return
			case <-t.C:
				if ok, err := l.RenewLease(runCtx, p.Name, tok, nowMs(), nowMs()+p.Lease.Milliseconds()); err != nil || !ok {
					cancel() // the lease is gone: stop working
					return
				}
			}
		}
	}()
	runErr := fn(runCtx)
	close(stopped)
	bg := context.WithoutCancel(ctx)
	if runErr != nil {
		_ = l.ReleaseLease(bg, p.Name, tok)
		if runCtx.Err() != nil && ctx.Err() == nil {
			return ErrLeaseLost
		}
		return runErr
	}
	if after == nil {
		return l.ReleaseLease(bg, p.Name, tok)
	}
	if ok, err := after(bg, tok); err != nil {
		return err
	} else if !ok {
		return ErrLeaseLost
	}
	return nil
}

// RunPeriod runs p for period once: a completed period, or another run holding the lease, has no effect.
func RunPeriod(ctx context.Context, l Leaser, owner string, p Periodic, period string) error {
	if done, err := l.PeriodDone(ctx, p.Name, period); err != nil || done {
		return err
	}
	return withLease(ctx, l, owner, p,
		func(ctx context.Context) (bool, error) { return l.PeriodDone(ctx, p.Name, period) }, // re-check under the lease
		func(ctx context.Context) error { return p.Run(context.WithValue(ctx, periodKey{}, period), period) },
		func(ctx context.Context, tok string) (bool, error) {
			return l.CompletePeriod(ctx, p.Name, period, tok, nowMs())
		})
}

// RunNow runs p outside its schedule (an administrator's "purge now"), exclusive with scheduled runs.
func RunNow(ctx context.Context, l Leaser, owner string, p Periodic) error {
	return withLease(ctx, l, owner, p, nil, func(ctx context.Context) error {
		return p.Run(context.WithValue(ctx, periodKey{}, "manual"), "manual")
	}, nil)
}

type periodPayload struct {
	Period string `json:"period"`
}

func taskType(p Periodic) string { return "periodic:" + p.Name }

// Schedule registers the periodic jobs with the queue (call before Start).
func (q *Queue) Schedule(l Leaser, owner string, defs []Periodic) {
	q.periodic, q.leaser, q.owner = defs, l, owner
	for _, p := range defs {
		q.Register(taskType(p), func(ctx context.Context, payload []byte) error {
			var pp periodPayload
			if err := json.Unmarshal(payload, &pp); err != nil || pp.Period == "" {
				return errors.Join(errors.New("bad periodic payload"), asynq.SkipRetry)
			}
			return RunPeriod(ctx, l, owner, p, pp.Period)
		})
	}
}

// Tick enqueues the current period of every periodic job when the queue is up (TaskID per period: other replicas'
// enqueues of the same period are ignored), and otherwise runs due periods in-process with the same leases.
func (q *Queue) Tick(ctx context.Context, now time.Time) {
	for _, p := range q.periodic {
		period := Period(now, p.Every)
		if q.Health().State == Up {
			payload, _ := json.Marshal(periodPayload{period})
			err := q.Enqueue(ctx, p.Queue, taskType(p), payload, asynq.TaskID(p.Name+":"+period),
				asynq.Retention(48*time.Hour), asynq.MaxRetry(5), asynq.Timeout(p.Lease*10))
			if err == nil || errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
				continue
			}
			q.log.Warn("enqueue periodic job", "job", p.Name, "err", err)
		}
		// in-process fallback: a failed job waits 5 minutes before the next attempt
		if t, failed := q.failedAt[p.Name]; failed && now.Sub(t) < 5*time.Minute {
			continue
		}
		if err := RunPeriod(ctx, q.leaser, q.owner, p, period); err != nil && !errors.Is(err, ErrBusy) {
			q.failedAt[p.Name] = now
			q.log.Error("periodic job", "job", p.Name, "period", period, "err", err)
		} else {
			delete(q.failedAt, p.Name)
		}
	}
}

// RunScheduler ticks every 30 seconds until ctx ends (and once right away, so a missed period runs at start).
func (q *Queue) RunScheduler(ctx context.Context) {
	q.Tick(ctx, time.Now())
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			q.Tick(ctx, now)
		}
	}
}

// Periodic returns a registered periodic job by name.
func (q *Queue) Periodic(name string) (Periodic, bool) {
	for _, p := range q.periodic {
		if p.Name == name {
			return p, true
		}
	}
	return Periodic{}, false
}
