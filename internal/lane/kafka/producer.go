// Package kafka runs the lanes of internal/lane on Kafka, with franz-go.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/martialanouman/sms-platform/internal/catalog"
	"github.com/martialanouman/sms-platform/internal/lane"
)

// Producer writes to the topics its service declares as producer in the catalog (spec 09 §9.1, guard 2).
type Producer struct {
	cl     *kgo.Client
	topics []string
}

// NewProducer opens a producer of service on topics, refusing any topic where service is not a producer; opts
// carry the brokers and the transport.
func NewProducer(c catalog.Catalog, service string, topics []string, opts ...kgo.Opt) (*Producer, error) {
	for _, t := range topics {
		if !slices.Contains(c.Topics[t].Producers, service) {
			return nil, fmt.Errorf("lane: %s is not a producer of %s", service, t)
		}
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("lane: producer: %w", err)
	}
	return &Producer{cl: cl, topics: topics}, nil
}

// Produce writes one record and waits for its acknowledgement by every in-sync replica.
func (p *Producer) Produce(ctx context.Context, topic string, key, value []byte, h lane.Headers) error {
	if !slices.Contains(p.topics, topic) {
		return fmt.Errorf("lane: topic %s is not one of this producer's", topic)
	}
	if h.OriginTS.IsZero() {
		return errors.New("lane: a record needs its origin_ts")
	}
	return p.cl.ProduceSync(ctx, &kgo.Record{Topic: topic, Key: key, Value: value, Headers: toKafka(h.Raw())}).FirstErr()
}

// Close flushes nothing, every Produce being synchronous, and releases the client.
func (p *Producer) Close() { p.cl.Close() }

func toKafka(raw []lane.Header) []kgo.RecordHeader {
	out := make([]kgo.RecordHeader, len(raw))
	for i, h := range raw {
		out[i] = kgo.RecordHeader{Key: h.Key, Value: h.Value}
	}
	return out
}
