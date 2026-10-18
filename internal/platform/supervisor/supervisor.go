// Package supervisor runs the components of a service and drains them in the order docs/spec/10-exploitation.md §10.2
// sets: the drain hooks, DrainDelay, then the components one by one in the reverse order of their registration.
package supervisor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrDrainBudgetExceeded reports a component still running when DrainBudget ran out.
var ErrDrainBudgetExceeded = errors.New("drain budget exceeded")

type component struct {
	name string
	run  func(context.Context) error
}

// Ordered runs components until the context ends or one of them returns, then drains them all.
type Ordered struct {
	DrainDelay, DrainBudget time.Duration

	components []component
	hooks      []func()
}

// Add registers a component, which runs until its context is cancelled and then returns nil or context.Canceled.
func (o *Ordered) Add(name string, run func(context.Context) error) {
	o.components = append(o.components, component{name, run})
}

// OnDrain registers a hook run when the drain starts, before DrainDelay and before any component stops.
func (o *Ordered) OnDrain(hook func()) {
	o.hooks = append(o.hooks, hook)
}

// Run starts every component, drains them all when ctx ends or one returns, and reports the first error, then a
// budget overrun.
func (o *Ordered) Run(ctx context.Context, log *slog.Logger) error {
	n := len(o.components)
	cancels := make([]context.CancelFunc, n)
	dones := make([]chan struct{}, n)
	errs := make([]error, n)
	returned := make(chan int, n)
	for i, c := range o.components {
		cctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		cancels[i], dones[i] = cancel, make(chan struct{})
		go func() {
			defer close(dones[i])
			if err := c.run(cctx); err != nil && !errors.Is(err, context.Canceled) {
				errs[i] = fmt.Errorf("%s: %w", c.name, err)
			}
			returned <- i
		}()
	}

	var first error
	select {
	case <-ctx.Done():
		log.Info("draining", "cause", context.Cause(ctx))
	case i := <-returned:
		first = errs[i]
		if first == nil {
			first = fmt.Errorf("%s: stopped before drain", o.components[i].name)
		}
		log.Error("draining", "err", first)
	}
	for _, hook := range o.hooks {
		hook()
	}
	time.Sleep(o.DrainDelay)

	budget := time.NewTimer(o.DrainBudget)
	defer budget.Stop()
	for i := n - 1; i >= 0; i-- {
		cancels[i]()
		select {
		case <-dones[i]:
			if first == nil {
				first = errs[i]
			}
		case <-budget.C:
			for _, cancel := range cancels[:i] {
				cancel()
			}
			log.Error("drain budget exceeded", "component", o.components[i].name, "budget", o.DrainBudget)
			if first == nil {
				first = fmt.Errorf("%w after %s: %s still running", ErrDrainBudgetExceeded, o.DrainBudget, o.components[i].name)
			}
			return first
		}
	}
	return first
}
