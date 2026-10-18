// Package metrics guards the labels of the Prometheus metrics at registration (docs/spec/10-exploitation.md §10.1).
package metrics

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

var (
	refusedSegments = []string{"msisdn", "phone", "number", "addr", "recipient", "destination", "body", "text", "content", "payload"}
	refusedNames    = []string{"to", "from", "message_id", "msg_id", "trace_id", "span_id", "customer_group", "client_group"}
)

var snakeCase = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func refused(label string) bool {
	return !snakeCase.MatchString(label) || slices.Contains(refusedNames, label) ||
		slices.ContainsFunc(strings.Split(label, "_"), func(s string) bool {
			return slices.Contains(refusedSegments, strings.TrimSuffix(s, "s"))
		})
}

type guard struct {
	prometheus.Registerer
}

// Guard wraps r, the registry itself, so that it refuses a collector carrying a refused label, or one it cannot read.
func Guard(r prometheus.Registerer) prometheus.Registerer {
	return guard{r}
}

func (g guard) Register(c prometheus.Collector) error {
	if err := check(c); err != nil {
		return err
	}
	return g.Registerer.Register(c)
}

func (g guard) MustRegister(cs ...prometheus.Collector) {
	for _, c := range cs {
		if err := g.Register(c); err != nil {
			panic(err)
		}
	}
}

func check(c prometheus.Collector) error {
	descs := make(chan *prometheus.Desc)
	go func() {
		c.Describe(descs)
		close(descs)
	}()
	var errs []error
	n := 0
	for d := range descs {
		n++
		names, err := labels(d)
		if err != nil {
			errs = append(errs, err)
		}
		for _, name := range names {
			if refused(name) {
				errs = append(errs, fmt.Errorf("metrics: %s: label %q is refused (§10.1)", d, name))
			}
		}
	}
	if n == 0 {
		return errors.New("metrics: the collector describes nothing, its labels cannot be checked")
	}
	return errors.Join(errs...)
}

func labels(d *prometheus.Desc) ([]string, error) {
	for n := 0; n <= len(d.String()); n++ {
		m, err := prometheus.NewConstMetric(d, prometheus.UntypedValue, 0, make([]string, n)...)
		if err != nil {
			continue
		}
		var out dto.Metric
		if err := m.Write(&out); err != nil {
			return nil, err
		}
		var names []string
		for _, l := range out.GetLabel() {
			names = append(names, l.GetName())
		}
		return names, nil
	}
	return nil, fmt.Errorf("metrics: invalid %s", d)
}
