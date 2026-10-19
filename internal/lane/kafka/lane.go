package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/martialanouman/sms-platform/internal/catalog"
	smsv1 "github.com/martialanouman/sms-platform/internal/gen/sms/v1"
	"github.com/martialanouman/sms-platform/internal/lane"
	perrors "github.com/martialanouman/sms-platform/internal/platform/errors"
)

const drainCommitTimeout = 10 * time.Second

// Handlers is the business side of a lane.
type Handlers struct {
	// Handle handles one record.
	Handle func(context.Context, lane.Record) lane.Outcome
	// Expired handles a record older than its max_age, which is never handled.
	Expired func(context.Context, lane.Record) lane.Outcome
	// Category names the category of a record whose topic does not carry it.
	Category func(lane.Record) (string, error)
	// Limit lowers the max_age of a record when it returns a positive duration.
	Limit func(lane.Record) time.Duration
}

// Lane consumes one consumer group of the catalog, committing a record only once it is handled or dead-lettered.
type Lane struct {
	cl     *kgo.Client
	policy lane.Policy
	h      Handlers
	log    *slog.Logger
}

// NewLane joins group, refusing a group the runtime cannot serve; opts carry the brokers and the transport.
func NewLane(c catalog.Catalog, group string, h Handlers, log *slog.Logger, opts ...kgo.Opt) (*Lane, error) {
	p, err := lane.NewPolicy(c, group, h.Category)
	if err != nil {
		return nil, err
	}
	reset := kgo.NewOffset().AtStart()
	if p.StartsFromEnd() {
		reset = kgo.NewOffset().AtEnd()
	}
	cl, err := kgo.NewClient(append(opts,
		kgo.ConsumerGroup(p.Name()),
		kgo.ConsumeTopics(p.Topic()),
		kgo.DisableAutoCommit(),
		kgo.ConsumeResetOffset(reset),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	)...)
	if err != nil {
		return nil, fmt.Errorf("lane: group %s: %w", group, err)
	}
	return &Lane{cl: cl, policy: p, h: h, log: log.With("group", p.Name())}, nil
}

// Close leaves the group and releases the client.
func (l *Lane) Close() { l.cl.Close() }

// Run consumes until ctx ends, returning nil then, and an error on a fetch or commit fault or a breach of contract
// by the handlers, which commits nothing more.
func (l *Lane) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		krs, err := l.next(ctx)
		if err != nil {
			return err
		}
		for attempt := 0; len(krs) > 0; {
			done, failed, err := l.handle(ctx, krs)
			if err != nil {
				return err
			}
			if done > 0 {
				if err := l.commit(ctx, krs[:done]...); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return fmt.Errorf("lane: commit in group %s: %w", l.policy.Name(), err)
				}
			}
			krs = krs[done:]
			if failed.dep == "" {
				continue
			}
			attempt = nextAttempt(attempt, done > 0)
			// debt: #69
			if !l.replay(ctx, attempt, krs[0], failed) {
				return nil
			}
		}
	}
	return nil
}

// failure is the dependency that stopped a batch, and the code of its error.
type failure struct {
	dep  string
	code perrors.Code
}

// handle handles krs in order and returns how many are done, and the failure that stopped the batch, if any.
func (l *Lane) handle(ctx context.Context, krs []*kgo.Record) (int, failure, error) {
	for i, kr := range krs {
		out := l.outcome(ctx, kr)
		switch out.Kind() {
		case lane.KindOk, lane.KindRefuse:
		case lane.KindDegraded:
			if l.policy.Mode(out.Dependency()) != "fail_open" {
				return 0, failure{}, l.breach(kr, out)
			}
		case lane.KindPoison:
			if err := l.deadLetter(ctx, kr, out.Reason()); err != nil {
				return i, failure{dep: "kafka", code: code(err)}, nil
			}
		case lane.KindTransient:
			if l.policy.Mode(out.Dependency()) != "blocking" {
				return 0, failure{}, l.breach(kr, out)
			}
			return i, failure{dep: out.Dependency(), code: code(out.Err())}, nil
		}
	}
	return len(krs), failure{}, nil
}

func (l *Lane) outcome(ctx context.Context, kr *kgo.Record) lane.Outcome {
	rec := lane.Record{Topic: kr.Topic, Partition: kr.Partition, Offset: kr.Offset, Key: kr.Key, Value: kr.Value}
	for _, h := range kr.Headers {
		rec.Raw = append(rec.Raw, lane.Header{Key: h.Key, Value: h.Value})
	}
	var err error
	if rec.Headers, err = lane.ParseHeaders(rec.Raw); err != nil {
		return lane.Poison("unreadable_headers")
	}
	var lower time.Duration
	if l.h.Limit != nil {
		lower = l.h.Limit(rec)
	}
	expired, err := l.policy.Expired(rec, time.Now(), lower)
	if err != nil {
		return lane.Poison("unreadable_category")
	}
	if expired {
		return l.h.Expired(ctx, rec)
	}
	return l.h.Handle(ctx, rec)
}

func (l *Lane) breach(kr *kgo.Record, out lane.Outcome) error {
	return fmt.Errorf("lane: group %s, %s[%d]@%d: outcome %d on dependency %q, which the catalog does not declare in that mode",
		l.policy.Name(), kr.Topic, kr.Partition, kr.Offset, out.Kind(), out.Dependency())
}

func (l *Lane) deadLetter(ctx context.Context, kr *kgo.Record, reason string) error {
	d := &smsv1.DeadLetter{
		Payload:   kr.Value,
		Reason:    reason,
		At:        timestamppb.Now(),
		Topic:     kr.Topic,
		Partition: kr.Partition,
		Offset:    kr.Offset,
	}
	var common []kgo.RecordHeader
	for _, h := range kr.Headers {
		d.Headers = append(d.Headers, &smsv1.Header{Key: h.Key, Value: h.Value})
		if slices.Contains([]string{lane.HeaderOriginTS, lane.HeaderTraceID, lane.HeaderReplayCount, lane.HeaderReplayedAt}, h.Key) {
			common = append(common, h)
		}
	}
	value, err := proto.Marshal(d)
	if err != nil {
		return err
	}
	return l.cl.ProduceSync(ctx, &kgo.Record{Topic: l.policy.DeadLetter(), Key: kr.Key, Value: value, Headers: common}).FirstErr()
}

func (l *Lane) next(ctx context.Context) ([]*kgo.Record, error) {
	fetches := l.cl.PollFetches(ctx)
	if ctx.Err() != nil {
		return nil, nil
	}
	var ferr error
	fetches.EachError(func(_ string, _ int32, err error) {
		if ferr == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			ferr = err
		}
	})
	if ferr != nil {
		return nil, fmt.Errorf("lane: fetch in group %s: %w", l.policy.Name(), ferr)
	}
	return fetches.Records(), nil
}

// commit retries once on a detached context when the drain cancelled the first attempt: work done before a SIGTERM
// must not be redelivered.
func (l *Lane) commit(ctx context.Context, recs ...*kgo.Record) error {
	err := l.cl.CommitRecords(ctx, recs...)
	if err == nil || ctx.Err() == nil {
		return err
	}
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), drainCommitTimeout)
	defer cancel()
	return l.cl.CommitRecords(dctx, recs...)
}

// replay waits before the failed record is handled again, in place, and reports false when ctx ended the wait.
func (l *Lane) replay(ctx context.Context, attempt int, kr *kgo.Record, f failure) bool {
	if ctx.Err() != nil {
		return false
	}
	var delay time.Duration
	if attempt > 0 {
		delay = replayDelay(attempt - 1)
	}
	l.log.WarnContext(ctx, "lane: replaying in place", append(position(kr), "dependency", f.dep, "code", f.code, "idle_attempts", attempt, "delay", delay)...)
	if delay == 0 {
		return true
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// nextAttempt counts consecutive attempts that handled nothing; only those wait (step-285 of go-gateway).
func nextAttempt(attempt int, progressed bool) int {
	if progressed {
		return 0
	}
	return attempt + 1
}

// replayDelay doubles from one second to a 30-second ceiling, with ±20 % jitter.
func replayDelay(attempt int) time.Duration {
	d := min(time.Second<<min(attempt, 5), 30*time.Second)
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
}

func position(kr *kgo.Record) []any {
	return []any{"topic", kr.Topic, "partition", kr.Partition, "offset", kr.Offset}
}

func code(err error) perrors.Code {
	if c, ok := errors.AsType[perrors.Code](err); ok {
		return c
	}
	return perrors.Code("internal_error")
}
