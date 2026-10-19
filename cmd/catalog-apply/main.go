// Command catalog-apply brings the Kafka cluster to contracts/topics.yaml: topics, configs and ACLs, then exits.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/martialanouman/sms-platform/contracts"
	"github.com/martialanouman/sms-platform/internal/catalog"
	"github.com/martialanouman/sms-platform/internal/catalog/kafka"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/platform/observability"
)

var errNoBrokers = errors.New("KAFKA_BROKERS is empty")

func main() {
	infra, err := config.Load()
	log := observability.NewLogger(os.Stdout, "catalog-apply", infra.LogLevel)
	if err == nil {
		err = run(context.Background(), infra, log)
	}
	if err != nil {
		log.Error("catalog-apply failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, infra config.Infra, log *slog.Logger) error {
	if len(infra.KafkaBrokers) == 0 {
		return errNoBrokers
	}
	tlsConfig, err := infra.TLS.Client()
	if err != nil {
		return fmt.Errorf("kafka mTLS: %w", err)
	}
	c, err := catalog.Parse(contracts.Topics())
	if err != nil {
		return err
	}
	cl, err := kgo.NewClient(kgo.SeedBrokers(infra.KafkaBrokers...), kgo.DialTLSConfig(tlsConfig))
	if err != nil {
		return err
	}
	defer cl.Close()
	p, err := kafka.Sync(ctx, cl, c)
	if err != nil {
		return err
	}
	report(log, p)
	return nil
}

func report(log *slog.Logger, p catalog.Plan) {
	for _, name := range slices.Sorted(maps.Keys(p.Create)) {
		log.Info("topic created", "topic", name, "partitions", p.Create[name].Partitions)
	}
	for _, name := range slices.Sorted(maps.Keys(p.Extend)) {
		log.Info("topic extended", "topic", name, "partitions", p.Extend[name])
	}
	for _, name := range slices.Sorted(maps.Keys(p.Configure)) {
		log.Info("topic configured", "topic", name, "configs", p.Configure[name])
	}
	for _, a := range p.Add {
		log.Info("ACL added", "acl", a)
	}
	for _, a := range p.Remove {
		log.Info("ACL removed", "acl", a)
	}
	for _, name := range p.Unmanaged {
		log.Warn("topic outside the catalog", "topic", name)
	}
	if p.Empty() {
		log.Info("cluster already matches the catalog")
	}
}
