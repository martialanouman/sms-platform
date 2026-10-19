package kafka_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/martialanouman/sms-platform/contracts"
	"github.com/martialanouman/sms-platform/internal/catalog"
	"github.com/martialanouman/sms-platform/internal/catalog/kafka"
)

func cluster(t *testing.T) (*kgo.Client, *kadm.Client) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx := context.Background()
	k, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.5.0", testcontainers.WithEnv(map[string]string{
		"KAFKA_AUTHORIZER_CLASS_NAME": "org.apache.kafka.metadata.authorizer.StandardAuthorizer",
		"KAFKA_SUPER_USERS":           "User:ANONYMOUS",
	}))
	testcontainers.CleanupContainer(t, k)
	if err != nil {
		t.Fatal(err)
	}
	brokers, err := k.Brokers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers...), kgo.MetadataMinAge(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cl.Close)
	return cl, kadm.NewClient(cl)
}

func real(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := catalog.Parse(contracts.Topics())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func ptr(s string) *string { return &s }

func create(t *testing.T, adm *kadm.Client, name string, partitions int32, configs map[string]*string) {
	t.Helper()
	r, err := adm.CreateTopic(context.Background(), partitions, -1, configs, name)
	if err == nil {
		err = r.Err
	}
	if err != nil {
		t.Fatal(err)
	}
}

func acls(t *testing.T, adm *kadm.Client) []catalog.ACL {
	t.Helper()
	rs, err := adm.DescribeACLs(context.Background(), kadm.NewACLs().AnyResource().ResourcePatternType(kadm.ACLPatternAny).
		Operations(kadm.OpAny).Allow().Deny().AllowHosts().DenyHosts())
	if err != nil {
		t.Fatal(err)
	}
	var got []catalog.ACL
	for _, r := range rs {
		if r.Err != nil {
			t.Fatal(r.Err)
		}
		for _, d := range r.Described {
			got = append(got, catalog.ACL{Principal: d.Principal, Resource: d.Type.String(), Name: d.Name, Pattern: d.Pattern.String(),
				Operation: d.Operation.String(), Permission: d.Permission.String(), Host: d.Host})
		}
	}
	return got
}

func eventually(t *testing.T, check func() error) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		err := check()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func byName(a, b catalog.ACL) int {
	return strings.Compare(a.Principal+a.Resource+a.Name+a.Operation, b.Principal+b.Resource+b.Name+b.Operation)
}

func matches(t *testing.T, adm *kadm.Client, c catalog.Catalog) error {
	ctx := context.Background()
	want := c.DeadLetters()
	maps.Copy(want, c.Topics)
	topics, err := adm.ListTopics(ctx)
	if err != nil {
		return err
	}
	configs, err := adm.DescribeTopicConfigs(ctx, topics.Names()...)
	if err != nil {
		return err
	}
	var errs []error
	for name, topic := range want {
		if got := len(topics[name].Partitions); got != int(topic.Partitions) {
			errs = append(errs, fmt.Errorf("topic %s: %d partitions, want %d", name, got, topic.Partitions))
			continue
		}
		rc, err := configs.On(name, nil)
		if err != nil {
			errs = append(errs, fmt.Errorf("topic %s: %w", name, err))
			continue
		}
		live := map[string]string{}
		for _, cfg := range rc.Configs {
			live[cfg.Key] = cfg.MaybeValue()
		}
		for k, v := range topic.Configs() {
			if live[k] != v {
				errs = append(errs, fmt.Errorf("topic %s: %s = %q, want %q", name, k, live[k], v))
			}
		}
		if name == "mt.inbound.otp" && live["min.insync.replicas"] != "2" {
			errs = append(errs, fmt.Errorf("the operator's min.insync.replicas of mt.inbound.otp is %q, want it kept at 2", live["min.insync.replicas"]))
		}
	}
	got, wantACLs := acls(t, adm), c.ACLs()
	slices.SortFunc(got, byName)
	slices.SortFunc(wantACLs, byName)
	if !reflect.DeepEqual(got, wantACLs) {
		errs = append(errs, fmt.Errorf("cluster ACLs differ from the catalog: %d against %d", len(got), len(wantACLs)))
	}
	return errors.Join(errs...)
}

func TestSyncBringsTheClusterToTheCatalog(t *testing.T) {
	cl, adm := cluster(t)
	ctx := context.Background()
	c := real(t)
	create(t, adm, "mt.inbound.otp", 4, map[string]*string{"retention.ms": ptr("1000"), "min.insync.replicas": ptr("2")})
	create(t, adm, "legacy", 1, nil)
	intruder := kadm.NewACLs().Topics("legacy").Allow("User:CN=intruder").Operations(kadm.OpRead)
	if _, err := adm.CreateACLs(ctx, intruder); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() error {
		topics, err := adm.ListTopics(ctx)
		if err == nil && (len(topics) != 2 || len(acls(t, adm)) != 1) {
			err = fmt.Errorf("the seed is not visible yet: %v", topics.Names())
		}
		return err
	})

	p, err := kafka.Sync(ctx, cl, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Remove) != 1 || p.Remove[0].Principal != "User:CN=intruder" {
		t.Errorf("removed %v, want the intruder's ACL", p.Remove)
	}
	eventually(t, func() error { return matches(t, adm, c) })

	p, err = kafka.Sync(ctx, cl, c)
	if err != nil || !p.Empty() || !reflect.DeepEqual(p.Unmanaged, []string{"legacy"}) {
		t.Errorf("second Sync = %+v, %v; want an empty plan that names legacy", p, err)
	}
}

func TestARefusedPlanWritesNothing(t *testing.T) {
	cl, adm := cluster(t)
	ctx := context.Background()
	create(t, adm, "mt.outcome", 4, nil)
	eventually(t, func() error {
		topics, err := adm.ListTopics(ctx)
		if err == nil && !topics.Has("mt.outcome") {
			err = errors.New("mt.outcome is not visible yet")
		}
		return err
	})

	if _, err := kafka.Sync(ctx, cl, real(t)); err == nil || !strings.Contains(err.Error(), "mt.outcome") {
		t.Fatalf("Sync = %v, want a refusal naming mt.outcome", err)
	}
	create(t, adm, "marker", 1, nil)
	var topics kadm.TopicDetails
	eventually(t, func() error {
		var err error
		if topics, err = adm.ListTopics(ctx); err == nil && !topics.Has("marker") {
			err = errors.New("the marker written after the refusal is not visible yet")
		}
		return err
	})
	if names := topics.Names(); !reflect.DeepEqual(names, []string{"marker", "mt.outcome"}) {
		t.Errorf("topics after a refusal = %v, want mt.outcome and the marker", names)
	}
	if got := acls(t, adm); len(got) > 0 {
		t.Errorf("ACLs after a refusal = %v, want none", got)
	}
}
