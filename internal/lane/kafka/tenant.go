package kafka

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/martialanouman/sms-platform/internal/lane"
)

// ponytail: one window for the whole pod; a slow tenant that fills it holds every partition (step 008b, arbitrage 6).
// Per-partition windows if a measure shows it.
const window = 1024

type slot struct {
	kr     *kgo.Record
	tenant string
	poison string
	done   bool
}

type result struct {
	s      *slot
	done   bool
	breach error
}

// runPerTenant hands the records of each tenant to at most MaxInFlight workers, reads on while fewer than window
// records are uncommitted, and commits each partition up to its oldest unfinished record.
func (l *Lane) runPerTenant(ctx context.Context) error {
	pctx, stop := context.WithCancel(ctx)
	batches, failed, polled := make(chan []*kgo.Record), make(chan error, 1), make(chan struct{})
	go func() {
		defer close(polled)
		for {
			krs, err := l.next(pctx)
			if err != nil {
				failed <- err
				return
			}
			if pctx.Err() != nil {
				return
			}
			select {
			case batches <- krs:
			case <-pctx.Done():
				return
			}
		}
	}()
	defer func() { stop(); <-polled }()

	pending := map[int32][]*slot{}
	queues := map[string][]*slot{}
	inFlight := map[string]int{}
	wctx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	results := make(chan result)
	held := 0
	var backlog []*kgo.Record
	var breach error
	dispatch := func(tenant string) {
		for inFlight[tenant] < l.policy.MaxInFlight() && len(queues[tenant]) > 0 {
			s := queues[tenant][0]
			queues[tenant] = queues[tenant][1:]
			inFlight[tenant]++
			go func() { results <- l.work(wctx, s) }()
		}
		if len(queues[tenant]) == 0 {
			delete(queues, tenant)
		}
	}
	done := ctx.Done()
	for {
		if breach != nil {
			cancelWork()
		}
		stopping := breach != nil || done == nil
		if stopping && len(inFlight) == 0 {
			break
		}
		for !stopping && held < window && len(backlog) > 0 {
			kr := backlog[0]
			backlog = backlog[1:]
			s := &slot{kr: kr}
			var err error
			if s.tenant, err = l.h.Tenant(record(kr)); err != nil {
				s.tenant, s.poison = "", "unreadable_tenant"
			}
			pending[kr.Partition] = append(pending[kr.Partition], s)
			queues[s.tenant] = append(queues[s.tenant], s)
			held++
			dispatch(s.tenant)
		}
		in := batches
		if stopping || len(backlog) > 0 {
			in = nil
		}
		select {
		case backlog = <-in:
		case r := <-results:
			if inFlight[r.s.tenant]--; inFlight[r.s.tenant] == 0 {
				delete(inFlight, r.s.tenant)
			}
			r.s.done = r.done
			if r.breach != nil && breach == nil {
				breach = r.breach
			}
			if breach == nil && done != nil {
				dispatch(r.s.tenant)
			}
		case err := <-failed:
			if breach == nil {
				breach = err
			}
		case <-done:
			done = nil
		}
		var prefix []*kgo.Record
		for p, slots := range pending {
			i := 0
			for i < len(slots) && slots[i].done {
				i++
			}
			if i > 0 {
				prefix = append(prefix, slots[i-1].kr)
				pending[p] = slots[i:]
				held -= i
			}
		}
		if len(prefix) == 0 {
			continue
		}
		if err := l.commit(ctx, prefix...); err != nil && ctx.Err() == nil && breach == nil {
			breach = fmt.Errorf("lane: commit in group %s: %w", l.policy.Name(), err)
		}
	}
	return breach
}

// work handles one record until it is done, replaying a blocking failure in place; it reports not done when ctx
// ended first.
func (l *Lane) work(ctx context.Context, s *slot) result {
	for attempt := 1; ; attempt++ {
		out, expired := lane.Poison(s.poison), false
		if s.poison == "" {
			out, expired = l.outcome(ctx, s.kr)
		}
		f, err := l.settle(ctx, s.kr, out, expired)
		if err != nil {
			return result{s: s, breach: err}
		}
		if f.dep == "" {
			return result{s: s, done: true}
		}
		// debt: #69
		if !l.replay(ctx, attempt, s.kr, f) {
			return result{s: s}
		}
	}
}
