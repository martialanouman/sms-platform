// Package metrics guards the labels of the Prometheus metrics at registration (docs/spec/10-exploitation.md §10.1).
package metrics

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	refusedSegments = []string{"msisdn", "phone", "number", "addr", "recipient", "destination", "body", "text", "content", "payload"}
	refusedNames    = []string{"to", "from", "message_id", "msg_id", "trace_id", "span_id", "customer_group", "client_group"}
)

var snakeCase = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func refused(label string) bool {
	return !snakeCase.MatchString(label) || slices.Contains(refusedNames, label) ||
		slices.ContainsFunc(strings.Split(label, "_"), func(s string) bool {
			return slices.Contains(refusedSegments, s) || slices.Contains(refusedSegments, strings.TrimSuffix(s, "s"))
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

func labels(desc string) ([]string, error) {
	unreadable := fmt.Errorf("metrics: unreadable %s", desc)
	rest := strings.TrimPrefix(desc, "Desc{")
	for _, field := range []string{"fqName: ", ", help: ", ", unit: "} {
		rest = strings.TrimPrefix(rest, field)
		quoted, err := strconv.QuotedPrefix(rest)
		if err != nil {
			return nil, unreadable
		}
		rest = rest[len(quoted):]
	}
	rest = strings.TrimPrefix(rest, ", constLabels: {")
	var names []string
	for !strings.HasPrefix(rest, "}") {
		name, value, ok := strings.Cut(rest, "=")
		quoted, err := strconv.QuotedPrefix(value)
		if !ok || err != nil {
			return nil, unreadable
		}
		names = append(names, name)
		rest = strings.TrimPrefix(value[len(quoted):], ",")
	}
	variables, prefixed := strings.CutPrefix(rest, "}, variableLabels: {")
	variables, suffixed := strings.CutSuffix(variables, "}}")
	if !prefixed || !suffixed {
		return nil, unreadable
	}
	if variables != "" {
		for _, l := range strings.Split(variables, ",") {
			names = append(names, strings.TrimSuffix(strings.TrimPrefix(l, "c("), ")"))
		}
	}
	return names, nil
}
