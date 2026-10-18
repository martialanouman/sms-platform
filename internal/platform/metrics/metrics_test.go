package metrics_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/martialanouman/sms-platform/internal/platform/metrics"
)

func counter(name string, labels []string, constLabels prometheus.Labels) prometheus.Collector {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: "h, variableLabels: {msisdn}", ConstLabels: constLabels}, labels)
}

func TestGuardAcceptsBoundedLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	g := metrics.Guard(reg)
	if err := g.Register(counter("sms_total", []string{"route_id", "consumer_group", "code", "context", "status"}, prometheus.Labels{"lane": "otp"})); err != nil {
		t.Fatal(err)
	}
	if err := g.Register(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "up_total", Help: "h"}, func() float64 { return 1 })); err != nil {
		t.Fatal(err)
	}
	if families, err := reg.Gather(); err != nil || len(families) != 1 {
		t.Fatalf("Gather = %d families, %v", len(families), err)
	}
}

func TestGuardRefusesForbiddenLabels(t *testing.T) {
	for _, label := range []string{
		"msisdn", "dest_msisdn", "PHONE", "phone_e164", "number", "source_addr", "recipient", "destination",
		"body", "body_len", "text", "content", "payload",
		"to", "from", "message_id", "msg_id", "trace_id", "span_id", "customer_group", "client_group",
		"msisdns", "recipients_count", "messageId", "phoneNumber", "msisdn}}", "lane ", "Route_id",
	} {
		t.Run(label, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			for name, c := range map[string]prometheus.Collector{
				"variable": counter("sms_total", []string{"route_id", label}, nil),
				"constant": counter("sms_total", nil, prometheus.Labels{label: "x"}),
			} {
				err := metrics.Guard(reg).Register(c)
				if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("label %q is refused", label)) {
					t.Errorf("%s label %q: Register = %v", name, label, err)
				}
				if reg.Unregister(c) {
					t.Errorf("%s label %q: the refused collector reached the registry", name, label)
				}
			}
		})
	}
}

func TestGuardRefusesAnUnreadableDesc(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := counter("sms_total", nil, prometheus.Labels{"msisdn": "1", "z=": "v"})
	if err := metrics.Guard(reg).Register(c); err == nil {
		t.Error("a Desc the guard cannot read registered")
	}
	if reg.Unregister(c) {
		t.Error("the unreadable collector reached the registry")
	}
}

type silent struct{}

func (silent) Describe(chan<- *prometheus.Desc) {}
func (silent) Collect(chan<- prometheus.Metric) {}

func TestGuardRefusesASilentCollector(t *testing.T) {
	if err := metrics.Guard(prometheus.NewRegistry()).Register(silent{}); err == nil {
		t.Error("a collector that describes nothing registered")
	}
}

func TestMustRegisterPanicsOnAForbiddenLabel(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustRegister accepted a forbidden label")
		}
	}()
	metrics.Guard(prometheus.NewRegistry()).MustRegister(counter("ok_total", nil, nil), counter("sms_total", []string{"msisdn"}, nil))
}

func TestDescRenderingIsPinned(t *testing.T) {
	d := prometheus.NewDesc("sms_total", "h", []string{"a", "b"}, prometheus.Labels{"c": "v"})
	if got, want := d.String(), `Desc{fqName: "sms_total", help: "h", unit: "", constLabels: {c="v"}, variableLabels: {a,b}}`; got != want {
		t.Errorf("Desc.String() = %s, want %s: the label guard reads this rendering", got, want)
	}
}
