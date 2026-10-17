package catalog_test

import (
	"strings"
	"testing"
	"time"

	"github.com/martialanouman/sms-platform/internal/catalog"
)

const valid = `
topics:
  mt.inbound.otp: &inbound
    key: message_id
    order: none
    duplicate_unit: message
    payload: sms.v1.MtInbound
    body: clear_bounded
    partitions: 64
    retention: 96h
    producers: [rest-api-svc]
  mt.inbound.marketing:
    <<: *inbound
  mt.outcome:
    key: message_id
    order: per_key
    duplicate_unit: transition
    payload: sms.v1.MtOutcome
    body: none
    partitions: 128
    retention: 96h
    producers: [router-svc]
  connector.account-status:
    key: connector_account_id
    order: per_key
    duplicate_unit: dernier état
    payload: sms.v1.ConnectorAccountStatus
    body: none
    partitions: 8
    compacted: true
    producers: [connector-pool-svc]
groups:
  - topic: mt.inbound.otp
    service: router-svc
    tenant: account_id
    dependencies:
      kafka: {mode: blocking}
      cancel-token: {mode: fail_open, justification: ADR-0013}
    concurrency: per_partition
    max_age: category
    dead_letter: router-svc.mt.inbound.otp.dead-letter
    start: earliest
  - topic: mt.outcome
    service: delivery-svc
    tenant: account_id
    dependencies:
      endpoint: {mode: park_by_tenant}
    concurrency: per_tenant
    max_in_flight: 8
    max_age: 24h
    dead_letter: delivery-svc.mt.outcome.dead-letter
    start: latest
    start_justification: lecture du flux courant seulement
  - topic: connector.account-status
    service: router-svc
    assignment: broadcast
    tenant: none
    dependencies:
      kafka: {mode: blocking}
    concurrency: per_partition
    dead_letter: router-svc.connector.account-status.dead-letter
    start: earliest
`

func parse(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := catalog.Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParseFollowsAnchors(t *testing.T) {
	c := parse(t)
	if got := c.Topics["mt.inbound.marketing"]; got.Partitions != 64 || got.Body != "clear_bounded" || got.Retention != 96*time.Hour {
		t.Errorf("merged topic = %+v", got)
	}
}

func TestParseRejectsUnknownFieldsAndDuplicateKeys(t *testing.T) {
	for name, doc := range map[string]string{
		"unknown field": "topics:\n  t:\n    partitons: 3\n",
		"duplicate key": "topics:\n  t:\n    key: a\n  t:\n    key: b\n",
	} {
		if _, err := catalog.Parse([]byte(doc)); err == nil {
			t.Errorf("%s: Parse accepted %q", name, doc)
		}
	}
}

func TestValidCatalogPasses(t *testing.T) {
	if errs := parse(t).Validate(); len(errs) > 0 {
		t.Errorf("Validate = %v", errs)
	}
}

func TestEveryGuardRejectsAFaultyEntry(t *testing.T) {
	topic := func(c *catalog.Catalog, name string, f func(*catalog.Topic)) {
		tp := c.Topics[name]
		f(&tp)
		c.Topics[name] = tp
	}
	cases := map[string]struct {
		mutate func(*catalog.Catalog)
		want   string
	}{
		"dead_letter missing":  {func(c *catalog.Catalog) { c.Groups[0].DeadLetter = "" }, "dead_letter"},
		"dead_letter misnamed": {func(c *catalog.Catalog) { c.Groups[0].DeadLetter = "router.dlq" }, "dead_letter"},
		"clear_bounded over 96h": {func(c *catalog.Catalog) {
			topic(c, "mt.inbound.otp", func(t *catalog.Topic) { t.Retention = 97 * time.Hour })
		}, "96h"},
		"retention missing":  {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Retention = 0 }) }, "retention"},
		"latest unjustified": {func(c *catalog.Catalog) { c.Groups[1].StartJustification = "" }, "start_justification"},
		"fail_open unjustified": {func(c *catalog.Catalog) {
			c.Groups[0].Dependencies["cancel-token"] = catalog.Dependency{Mode: "fail_open"}
		}, "justification"},
		"unknown order":              {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Order = "strict" }) }, "order"},
		"unknown body":               {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Body = "encrypted" }) }, "body"},
		"no partitions":              {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Partitions = 0 }) }, "partitions"},
		"no producers":               {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Producers = nil }) }, "producers"},
		"undeclared topic":           {func(c *catalog.Catalog) { c.Groups[0].Topic = "mt.nowhere" }, "mt.nowhere is not declared"},
		"duplicate group":            {func(c *catalog.Catalog) { c.Groups = append(c.Groups, c.Groups[0]) }, "duplicate"},
		"unknown dependency mode":    {func(c *catalog.Catalog) { c.Groups[0].Dependencies["kafka"] = catalog.Dependency{Mode: "retry"} }, "mode"},
		"unknown assignment":         {func(c *catalog.Catalog) { c.Groups[0].Assignment = "sticky" }, "assignment"},
		"unknown start":              {func(c *catalog.Catalog) { c.Groups[0].Start = "middle" }, "start"},
		"unknown concurrency":        {func(c *catalog.Catalog) { c.Groups[0].Concurrency = "per_key" }, "concurrency"},
		"per_tenant without ceiling": {func(c *catalog.Catalog) { c.Groups[1].MaxInFlight = 0 }, "max_in_flight"},
		"ceiling without per_tenant": {func(c *catalog.Catalog) { c.Groups[0].MaxInFlight = 4 }, "max_in_flight"},
		"bad max_age":                {func(c *catalog.Catalog) { c.Groups[0].MaxAge = "soon" }, "max_age"},
	}
	for name, tc := range cases {
		c := parse(t)
		tc.mutate(&c)
		errs := c.Validate()
		found := false
		for _, err := range errs {
			found = found || strings.Contains(err.Error(), tc.want)
		}
		if !found {
			t.Errorf("%s: Validate = %v, want an error mentioning %q", name, errs, tc.want)
		}
	}
}

func TestFrozenRefusesNewPartitionCountsOnPerKeyTopics(t *testing.T) {
	base, head := parse(t), parse(t)
	tp := head.Topics["mt.outcome"]
	tp.Partitions = 256
	head.Topics["mt.outcome"] = tp
	in := head.Topics["mt.inbound.otp"]
	in.Partitions = 128
	head.Topics["mt.inbound.otp"] = in
	head.Topics["new.topic"] = catalog.Topic{Order: "per_key", Partitions: 3}

	errs := catalog.Frozen(base, head)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "mt.outcome") {
		t.Errorf("Frozen = %v, want one error on mt.outcome", errs)
	}
	if errs := catalog.Frozen(base, parse(t)); len(errs) > 0 {
		t.Errorf("Frozen on an unchanged catalog = %v", errs)
	}
}

func TestDeadLettersAreDerivedFromTheirSource(t *testing.T) {
	dl := parse(t).DeadLetters()
	got, ok := dl["router-svc.mt.inbound.otp.dead-letter"]
	want := catalog.Topic{Key: "message_id", Order: "none", DuplicateUnit: "record", Payload: "sms.v1.MtInbound", Body: "clear_bounded", Partitions: 8, Retention: 96 * time.Hour, Producers: []string{"router-svc"}}
	if !ok || got.Key != want.Key || got.Order != want.Order || got.Body != want.Body || got.Partitions != want.Partitions || got.Retention != want.Retention || got.Payload != want.Payload || len(got.Producers) != 1 || got.Producers[0] != "router-svc" {
		t.Errorf("dead-letter = %+v, want %+v", got, want)
	}
	if len(dl) != 3 {
		t.Errorf("%d dead-letters, want one per group", len(dl))
	}
}

func TestMarkdownListsTopicsGroupsAndDeadLetters(t *testing.T) {
	page := string(parse(t).Markdown())
	for _, s := range []string{"mt.inbound.otp", "delivery-svc.mt.outcome", "router-svc.mt.inbound.otp.dead-letter", "broadcast", "fail_open"} {
		if !strings.Contains(page, s) {
			t.Errorf("page lacks %q", s)
		}
	}
}
