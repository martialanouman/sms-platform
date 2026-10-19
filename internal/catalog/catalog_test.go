package catalog_test

import (
	"reflect"
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
  mt.routed.otp:
    key: manual
    order: per_partition
    duplicate_unit: segment
    payload: sms.v1.MtRouted
    body: clear_bounded
    partitions: 384
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
  - topic: mt.inbound.otp
    service: router-svc
    lane: seal
    tenant: none
    dependencies:
      kafka: {mode: blocking}
    concurrency: per_partition
    dead_letter: router-svc.seal.mt.inbound.otp.dead-letter
    start: earliest
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
		"second document": "topics: {}\n---\ngroups: []\n",
		"unknown field":   "topics:\n  t:\n    partitons: 3\n",
		"duplicate key":   "topics:\n  t:\n    key: a\n  t:\n    key: b\n",
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
		"per_partition without manual key": {func(c *catalog.Catalog) { topic(c, "mt.routed.otp", func(t *catalog.Topic) { t.Key = "message_id" }) }, "manual"},
		"manual key without per_partition": {func(c *catalog.Catalog) { topic(c, "mt.routed.otp", func(t *catalog.Topic) { t.Order = "none" }) }, "manual"},
		"empty service":                    {func(c *catalog.Catalog) { c.Groups[0].Service = "" }, "service"},
		"empty key":                        {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Key = "" }) }, "key"},
		"empty payload":                    {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Payload = "" }) }, "payload"},
		"empty duplicate_unit":             {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.DuplicateUnit = "" }) }, "duplicate_unit"},
		"unknown tenant":                   {func(c *catalog.Catalog) { c.Groups[0].Tenant = "customer" }, "tenant"},
		"per_tenant without a tenant": {func(c *catalog.Catalog) {
			c.Groups[1].Tenant = "none"
			c.Groups[1].Dependencies = map[string]catalog.Dependency{"kafka": {Mode: "blocking"}}
		}, "per_tenant needs a tenant"},
		"park_by_tenant without a tenant": {func(c *catalog.Catalog) {
			c.Groups[0].Tenant = "none"
			c.Groups[0].Dependencies["endpoint"] = catalog.Dependency{Mode: "park_by_tenant"}
		}, "park_by_tenant dependency endpoint needs a tenant"},
		"no dependencies":                    {func(c *catalog.Catalog) { c.Groups[0].Dependencies = nil }, "dependencies"},
		"manual assignment off a manual key": {func(c *catalog.Catalog) { c.Groups[0].Assignment = "manual" }, "manual"},
		"unknown order":                      {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Order = "strict" }) }, "order"},
		"unknown body":                       {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Body = "encrypted" }) }, "body"},
		"no partitions":                      {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Partitions = 0 }) }, "partitions"},
		"no producers":                       {func(c *catalog.Catalog) { topic(c, "mt.outcome", func(t *catalog.Topic) { t.Producers = nil }) }, "producers"},
		"undeclared topic":                   {func(c *catalog.Catalog) { c.Groups[0].Topic = "mt.nowhere" }, "mt.nowhere is not declared"},
		"duplicate group":                    {func(c *catalog.Catalog) { c.Groups = append(c.Groups, c.Groups[0]) }, "duplicate"},
		"unknown dependency mode":            {func(c *catalog.Catalog) { c.Groups[0].Dependencies["kafka"] = catalog.Dependency{Mode: "retry"} }, "mode"},
		"unknown assignment":                 {func(c *catalog.Catalog) { c.Groups[0].Assignment = "sticky" }, "assignment"},
		"unknown start":                      {func(c *catalog.Catalog) { c.Groups[0].Start = "middle" }, "start"},
		"unknown concurrency":                {func(c *catalog.Catalog) { c.Groups[0].Concurrency = "per_key" }, "concurrency"},
		"per_tenant without ceiling":         {func(c *catalog.Catalog) { c.Groups[1].MaxInFlight = 0 }, "max_in_flight"},
		"ceiling without per_tenant":         {func(c *catalog.Catalog) { c.Groups[0].MaxInFlight = 4 }, "max_in_flight"},
		"bad max_age":                        {func(c *catalog.Catalog) { c.Groups[0].MaxAge = "soon" }, "max_age"},
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
	status := head.Topics["connector.account-status"]
	status.Order = "none"
	head.Topics["connector.account-status"] = status
	outcome := head.Topics["mt.outcome"]
	outcome.Key = "account_id"
	head.Topics["mt.outcome"] = outcome
	routed := head.Topics["mt.routed.otp"]
	routed.Partitions = 448
	head.Topics["mt.routed.otp"] = routed

	errs := catalog.Frozen(base, head)
	if len(errs) != 2 || !strings.Contains(errs[0].Error(), "connector.account-status") || !strings.Contains(errs[1].Error(), "mt.outcome") {
		t.Errorf("Frozen = %v, want errors on connector.account-status (order) and mt.outcome (partitions)", errs)
	}
	keyOnly := parse(t)
	k := keyOnly.Topics["mt.outcome"]
	k.Key = "account_id"
	keyOnly.Topics["mt.outcome"] = k
	if errs := catalog.Frozen(base, keyOnly); len(errs) != 1 {
		t.Errorf("Frozen on a changed key = %v, want one error", errs)
	}
	if errs := catalog.Frozen(base, parse(t)); len(errs) > 0 {
		t.Errorf("Frozen on an unchanged catalog = %v", errs)
	}
}

func TestDeadLettersAreDerivedFromTheirSource(t *testing.T) {
	dl := parse(t).DeadLetters()
	want := catalog.Topic{Key: "message_id", Order: "none", Payload: "sms.v1.DeadLetter", Body: "clear_bounded", Partitions: 8, Retention: 96 * time.Hour}
	if got := dl["router-svc.mt.inbound.otp.dead-letter"]; !reflect.DeepEqual(got, want) {
		t.Errorf("dead-letter = %+v, want %+v", got, want)
	}
	if len(dl) != 4 {
		t.Errorf("%d dead-letters, want one per group", len(dl))
	}
}

func TestMarkdownListsTopicsGroupsAndDeadLetters(t *testing.T) {
	page := string(parse(t).Markdown())
	for _, s := range []string{"mt.inbound.otp", "delivery-svc.mt.outcome", "router-svc.mt.inbound.otp.dead-letter", "router-svc.seal.mt.inbound.otp", "broadcast", "fail_open"} {
		if !strings.Contains(page, s) {
			t.Errorf("page lacks %q", s)
		}
	}
}

func TestMissingPayloadsNamesEveryUnknownMessage(t *testing.T) {
	known := map[string]bool{"sms.v1.MtInbound": true, "sms.v1.MtRouted": true, "sms.v1.ConnectorAccountStatus": true, "sms.v1.DeadLetter": true}
	errs := parse(t).MissingPayloads(func(name string) bool { return known[name] })
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "sms.v1.MtOutcome") || !strings.Contains(errs[0].Error(), "mt.outcome") {
		t.Errorf("MissingPayloads = %v, want one error naming mt.outcome and sms.v1.MtOutcome", errs)
	}
	if errs := parse(t).MissingPayloads(func(string) bool { return true }); len(errs) > 0 {
		t.Errorf("MissingPayloads with every message known = %v", errs)
	}
	errs = parse(t).MissingPayloads(func(name string) bool { return name != "sms.v1.DeadLetter" })
	if len(errs) != 4 || !strings.Contains(errs[0].Error(), ".dead-letter") {
		t.Errorf("MissingPayloads without sms.v1.DeadLetter = %v, want one error per dead-letter", errs)
	}
}

func acl(service, resource, name, operation string) catalog.ACL {
	return catalog.ACL{Principal: "User:CN=" + service, Resource: resource, Name: name, Pattern: "LITERAL", Operation: operation, Permission: "ALLOW", Host: "*"}
}

func TestACLsDeriveFromProducersAndGroups(t *testing.T) {
	want := []catalog.ACL{
		acl("connector-pool-svc", "TOPIC", "connector.account-status", "WRITE"),
		acl("delivery-svc", "GROUP", "delivery-svc.mt.outcome", "READ"),
		acl("delivery-svc", "TOPIC", "delivery-svc.mt.outcome.dead-letter", "WRITE"),
		acl("delivery-svc", "TOPIC", "mt.outcome", "READ"),
		acl("rest-api-svc", "TOPIC", "mt.inbound.marketing", "WRITE"),
		acl("rest-api-svc", "TOPIC", "mt.inbound.otp", "WRITE"),
		acl("router-svc", "GROUP", "router-svc.connector.account-status", "READ"),
		acl("router-svc", "GROUP", "router-svc.mt.inbound.otp", "READ"),
		acl("router-svc", "GROUP", "router-svc.seal.mt.inbound.otp", "READ"),
		acl("router-svc", "TOPIC", "connector.account-status", "READ"),
		acl("router-svc", "TOPIC", "mt.inbound.otp", "READ"),
		acl("router-svc", "TOPIC", "mt.outcome", "WRITE"),
		acl("router-svc", "TOPIC", "mt.routed.otp", "WRITE"),
		acl("router-svc", "TOPIC", "router-svc.connector.account-status.dead-letter", "WRITE"),
		acl("router-svc", "TOPIC", "router-svc.mt.inbound.otp.dead-letter", "WRITE"),
		acl("router-svc", "TOPIC", "router-svc.seal.mt.inbound.otp.dead-letter", "WRITE"),
	}
	if got := parse(t).ACLs(); !reflect.DeepEqual(got, want) {
		t.Errorf("ACLs() =\n%v\nwant\n%v", got, want)
	}
}

func TestConfigsSetTheCleanupPolicyAndTheRetentionOfADeletedTopic(t *testing.T) {
	c := parse(t)
	if got, want := c.Topics["mt.outcome"].Configs(), map[string]string{"cleanup.policy": "delete", "retention.ms": "345600000"}; !reflect.DeepEqual(got, want) {
		t.Errorf("mt.outcome Configs() = %v, want %v", got, want)
	}
	if got, want := c.Topics["connector.account-status"].Configs(), map[string]string{"cleanup.policy": "compact"}; !reflect.DeepEqual(got, want) {
		t.Errorf("connector.account-status Configs() = %v, want %v", got, want)
	}
}

func applied(c catalog.Catalog) catalog.State {
	s := catalog.State{Topics: map[string]catalog.Live{}, ACLs: c.ACLs()}
	for _, topics := range []map[string]catalog.Topic{c.Topics, c.DeadLetters()} {
		for name, t := range topics {
			s.Topics[name] = catalog.Live{Partitions: int(t.Partitions), Configs: t.Configs()}
		}
	}
	return s
}

func TestPlanOfAnEmptyClusterCreatesEverything(t *testing.T) {
	c := parse(t)
	p, err := c.Plan(catalog.State{})
	if err != nil {
		t.Fatal(err)
	}
	want := applied(c)
	if len(p.Create) != len(want.Topics) {
		t.Errorf("creates %d topics, want %d", len(p.Create), len(want.Topics))
	}
	for name, live := range want.Topics {
		if got := p.Create[name]; int(got.Partitions) != live.Partitions || !reflect.DeepEqual(got.Configs(), live.Configs) {
			t.Errorf("create %s = %+v, want %+v", name, got, live)
		}
	}
	if !reflect.DeepEqual(p.Add, c.ACLs()) || len(p.Remove)+len(p.Extend)+len(p.Configure) > 0 {
		t.Errorf("plan = %+v, want every ACL added and nothing else", p)
	}
}

func TestPlanOfAnAppliedClusterIsEmpty(t *testing.T) {
	c := parse(t)
	s := applied(c)
	s.Topics["mt.outcome"].Configs["min.insync.replicas"] = "2"
	s.Topics["mt.routed.otp"] = catalog.Live{Partitions: 448, Configs: s.Topics["mt.routed.otp"].Configs}
	p, err := c.Plan(s)
	if err != nil || !p.Empty() {
		t.Errorf("Plan = %+v, %v; want an empty plan", p, err)
	}
}

func TestEmptyIsAPlanThatWritesNothing(t *testing.T) {
	a := []catalog.ACL{acl("router-svc", "TOPIC", "mt.outcome", "WRITE")}
	for _, p := range []catalog.Plan{
		{Create: map[string]catalog.Topic{"t": {}}},
		{Extend: map[string]int{"t": 1}},
		{Configure: map[string]map[string]string{"t": {}}},
		{Add: a},
		{Remove: a},
	} {
		if p.Empty() {
			t.Errorf("%+v is empty", p)
		}
	}
	if !(catalog.Plan{Unmanaged: []string{"legacy"}}).Empty() {
		t.Error("a plan that only reports an unmanaged topic is not empty")
	}
}

func TestPlanExtendsAndAlignsWhatTheCatalogDemands(t *testing.T) {
	c := parse(t)
	s := applied(c)
	for _, name := range []string{"mt.routed.otp", "mt.inbound.otp", "router-svc.mt.inbound.otp.dead-letter"} {
		s.Topics[name] = catalog.Live{Partitions: 4, Configs: s.Topics[name].Configs}
	}
	s.Topics["mt.outcome"] = catalog.Live{Partitions: 128, Configs: map[string]string{"cleanup.policy": "delete", "retention.ms": "1000"}}
	s.Topics["connector.account-status"] = catalog.Live{Partitions: 8, Configs: map[string]string{"cleanup.policy": "delete", "retention.ms": "1000"}}
	stray := acl("router-svc", "TOPIC", "mt.routed.otp", "READ")
	s.ACLs = append(s.ACLs[1:], stray)
	s.Topics["legacy"] = catalog.Live{Partitions: 1}
	p, err := c.Plan(s)
	if err != nil {
		t.Fatal(err)
	}
	want := catalog.Plan{
		Extend:    map[string]int{"mt.routed.otp": 384, "mt.inbound.otp": 64, "router-svc.mt.inbound.otp.dead-letter": 8},
		Configure: map[string]map[string]string{"mt.outcome": {"retention.ms": "345600000"}, "connector.account-status": {"cleanup.policy": "compact"}},
		Add:       c.ACLs()[:1],
		Remove:    []catalog.ACL{stray},
		Unmanaged: []string{"legacy"},
	}
	if !reflect.DeepEqual(p, want) {
		t.Errorf("Plan =\n%+v\nwant\n%+v", p, want)
	}
}

func TestPlanRefusesEveryPartitionCountKafkaCannotReach(t *testing.T) {
	c := parse(t)
	c.Groups = append(c.Groups, catalog.Group{Topic: "mt.routed.otp", Service: "billing-svc", DeadLetter: "billing-svc.mt.routed.otp.dead-letter"})
	s := applied(c)
	for name, partitions := range map[string]int{"mt.outcome": 64, "connector.account-status": 16, "mt.inbound.otp": 65, "router-svc.mt.inbound.otp.dead-letter": 9, "billing-svc.mt.routed.otp.dead-letter": 9} {
		s.Topics[name] = catalog.Live{Partitions: partitions, Configs: s.Topics[name].Configs}
	}
	delete(s.Topics, "mt.inbound.marketing")
	s.Topics["mt.routed.otp"] = catalog.Live{Partitions: 4}
	s.Topics["legacy"] = catalog.Live{Partitions: 1}
	s.ACLs = nil
	p, err := c.Plan(s)
	if err == nil || !reflect.DeepEqual(p, catalog.Plan{}) {
		t.Fatalf("Plan = %+v, %v; want a refusal and no plan", p, err)
	}
	for _, name := range []string{"mt.outcome", "connector.account-status", "mt.inbound.otp:", "router-svc.mt.inbound.otp.dead-letter", "billing-svc.mt.routed.otp.dead-letter"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("refusal does not name %s: %v", name, err)
		}
	}
}

func TestMarkdownListsTheACLsAndTheInitialSizeOfAManualTopic(t *testing.T) {
	page := string(parse(t).Markdown())
	for _, s := range []string{"| `mt.routed.otp` | `manual` | per_partition | clear_bounded | 384 (initial) |", "| `mt.outcome` | `message_id` | per_key | none | 128 | 96h |", "| `User:CN=router-svc` | TOPIC | `mt.routed.otp` | WRITE |"} {
		if !strings.Contains(page, s) {
			t.Errorf("page lacks %q", s)
		}
	}
}
