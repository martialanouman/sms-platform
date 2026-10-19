package kafka_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/martialanouman/sms-platform/internal/catalog"
	"github.com/martialanouman/sms-platform/internal/lane"
	"github.com/martialanouman/sms-platform/internal/lane/kafka"
)

var brokers []string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()
	k, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.5.0")
	if k != nil {
		defer func() { _ = testcontainers.TerminateContainer(k) }()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if brokers, err = k.Brokers(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

var seq atomic.Int64

func unique() string {
	return fmt.Sprintf("x%d%d", time.Now().UnixNano()%1e6, seq.Add(1))
}

func client(t *testing.T, opts ...kgo.Opt) *kgo.Client {
	t.Helper()
	cl, err := kgo.NewClient(append([]kgo.Opt{kgo.SeedBrokers(brokers...)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cl.Close)
	return cl
}

func createTopics(t *testing.T, partitions int32, names ...string) {
	t.Helper()
	resp, err := kadm.NewClient(client(t)).CreateTopics(context.Background(), partitions, 1, nil, names...)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Error(); err != nil {
		t.Fatal(err)
	}
}

func readAll(t *testing.T, topic string, n int) []*kgo.Record {
	t.Helper()
	cl := client(t, kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var out []*kgo.Record
	for len(out) < n {
		fs := cl.PollFetches(ctx)
		if ctx.Err() != nil {
			t.Fatalf("read %d of %d records on %s", len(out), n, topic)
		}
		out = append(out, fs.Records()...)
	}
	return out
}

// world is a catalog of one source topic, one producer and one group, under fresh names.
type world struct {
	cat     catalog.Catalog
	topic   string
	group   string
	dl      string
	service string
}

func newWorld(t *testing.T, groupFields string) world {
	t.Helper()
	n := unique()
	w := world{topic: n + ".mt.inbound.otp", service: n + "-svc"}
	w.group = w.service + "." + w.topic
	w.dl = w.group + ".dead-letter"
	c, err := catalog.Parse([]byte(fmt.Sprintf(`
topics:
  %[1]s: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.MtInbound, body: clear_bounded, partitions: 2, retention: 96h, producers: [%[2]s]}
  other: {key: message_id, order: none, duplicate_unit: m, payload: sms.v1.MtInbound, body: none, partitions: 2, retention: 96h, producers: [someone-else]}
groups:
  - {topic: %[1]s, service: %[2]s, tenant: none, concurrency: per_partition, dead_letter: %[3]s, start: earliest, %[4]s}
`, w.topic, w.service, w.dl, groupFields)))
	if err != nil {
		t.Fatal(err)
	}
	if errs := c.Validate(); len(errs) > 0 {
		t.Fatal(errs)
	}
	w.cat = c
	createTopics(t, 2, w.topic, w.dl)
	return w
}

func TestProducerRefusesATopicItsServiceDoesNotProduce(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}}")
	for name, topics := range map[string][]string{
		"another service's topic": {"other"},
		"an undeclared topic":     {"nowhere"},
	} {
		if p, err := kafka.NewProducer(w.cat, w.service, topics, kgo.SeedBrokers(brokers...)); err == nil {
			p.Close()
			t.Errorf("%s: NewProducer accepted %v", name, topics)
		}
	}
}

func TestProducerWritesTheCommonHeaders(t *testing.T) {
	w := newWorld(t, "dependencies: {kafka: {mode: blocking}}")
	p, err := kafka.NewProducer(w.cat, w.service, []string{w.topic}, kgo.SeedBrokers(brokers...))
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	ctx := context.Background()
	h := lane.Headers{OriginTS: time.Now().UTC().Truncate(time.Microsecond), TraceID: "abc", ReplayCount: 1}
	if err := p.Produce(ctx, w.topic, []byte("k"), []byte("v"), h); err != nil {
		t.Fatal(err)
	}
	if err := p.Produce(ctx, w.dl, []byte("k"), []byte("v"), h); err == nil {
		t.Error("Produce accepted a topic outside its list")
	}
	if err := p.Produce(ctx, w.topic, []byte("k"), []byte("v"), lane.Headers{}); err == nil {
		t.Error("Produce accepted a record without origin_ts")
	}
	recs := readAll(t, w.topic, 1)
	if len(recs) != 1 || string(recs[0].Key) != "k" || string(recs[0].Value) != "v" {
		t.Fatalf("records = %v", recs)
	}
	var raw []lane.Header
	for _, x := range recs[0].Headers {
		raw = append(raw, lane.Header{Key: x.Key, Value: x.Value})
	}
	got, err := lane.ParseHeaders(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.OriginTS.Equal(h.OriginTS) || got.TraceID != h.TraceID || got.ReplayCount != h.ReplayCount {
		t.Errorf("headers = %+v, want %+v", got, h)
	}
}
