// Package metrics guards the labels of the Prometheus metrics at registration (docs/spec/10-exploitation.md §10.1).
package metrics

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	refusedSegments = []string{"msisdn", "phone", "number", "addr", "recipient", "destination", "body", "text", "content", "payload"}
	refusedNames    = []string{"to", "from", "message_id", "msg_id", "trace_id", "span_id", "customer_group", "client_group"}
)

func refused(label string) bool {
	label = strings.ToLower(label)
	return slices.Contains(refusedNames, label) || slices.ContainsFunc(strings.Split(label, "_"), func(s string) bool {
		return slices.Contains(refusedSegments, s)
	})
}

type guard struct {
	prometheus.Registerer
}

// Guard wraps r so that it refuses a collector carrying a refused label, or one it cannot read.
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
		names, err := labels(d.String())
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

const (
	constMark    = ", constLabels: {"
	variableMark = ", variableLabels: {"
)

func labels(desc string) ([]string, error) {
	v := strings.LastIndex(desc, variableMark)
	c := strings.LastIndex(desc[:max(v, 0)], constMark)
	if c < 0 || !strings.HasSuffix(desc, "}}") {
		return nil, fmt.Errorf("metrics: unreadable %s", desc)
	}
	var names []string
	for rest := strings.TrimSuffix(desc[c+len(constMark):v], "}"); rest != ""; {
		name, value, ok := strings.Cut(rest, "=")
		quoted, err := strconv.QuotedPrefix(value)
		if !ok || err != nil {
			return nil, fmt.Errorf("metrics: unreadable constant labels in %s", desc)
		}
		names = append(names, name)
		rest = strings.TrimPrefix(value[len(quoted):], ",")
	}
	if variables := desc[v+len(variableMark) : len(desc)-2]; variables != "" {
		for _, l := range strings.Split(variables, ",") {
			names = append(names, strings.TrimSuffix(strings.TrimPrefix(l, "c("), ")"))
		}
	}
	return names, nil
}
