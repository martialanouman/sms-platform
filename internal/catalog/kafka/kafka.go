// Package kafka brings a Kafka cluster to the topic catalog through the admin API.
package kafka

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/martialanouman/sms-platform/internal/catalog"
)

// Sync reads the cluster, plans it against the catalog and applies the plan; a refused plan writes nothing.
func Sync(ctx context.Context, cl *kgo.Client, c catalog.Catalog) (catalog.Plan, error) {
	adm := kadm.NewClient(cl)
	s, err := read(ctx, cl, adm)
	if err != nil {
		return catalog.Plan{}, err
	}
	p, err := c.Plan(s)
	if err != nil {
		return catalog.Plan{}, err
	}
	return p, apply(ctx, cl, adm, p)
}

func read(ctx context.Context, cl *kgo.Client, adm *kadm.Client) (catalog.State, error) {
	topics, err := adm.ListTopics(ctx)
	if err == nil {
		err = topics.Error()
	}
	if err != nil {
		return catalog.State{}, fmt.Errorf("list topics: %w", err)
	}
	configs, err := adm.DescribeTopicConfigs(ctx, topics.Names()...)
	if err != nil {
		return catalog.State{}, fmt.Errorf("describe topic configs: %w", err)
	}
	s := catalog.State{Topics: map[string]catalog.Live{}}
	for _, rc := range configs {
		if rc.Err != nil {
			return catalog.State{}, fmt.Errorf("topic %s: %w", rc.Name, rc.Err)
		}
		live := catalog.Live{Partitions: len(topics[rc.Name].Partitions), Configs: map[string]string{}}
		for _, cfg := range rc.Configs {
			live.Configs[cfg.Key] = cfg.MaybeValue()
		}
		s.Topics[rc.Name] = live
	}
	req := kmsg.NewPtrDescribeACLsRequest()
	req.ResourceType = kmsg.ACLResourceTypeAny
	req.ResourcePatternType = kmsg.ACLResourcePatternTypeAny
	req.Operation = kmsg.ACLOperationAny
	req.PermissionType = kmsg.ACLPermissionTypeAny
	resp, err := req.RequestWith(ctx, cl)
	if err == nil {
		err = kerr.ErrorForCode(resp.ErrorCode)
	}
	if err != nil {
		return catalog.State{}, fmt.Errorf("describe ACLs: %w", err)
	}
	for _, r := range resp.Resources {
		for _, a := range r.ACLs {
			s.ACLs = append(s.ACLs, catalog.ACL{Principal: a.Principal, Resource: r.ResourceType.String(), Name: r.ResourceName,
				Pattern: r.ResourcePatternType.String(), Operation: a.Operation.String(), Permission: a.PermissionType.String(), Host: a.Host})
		}
	}
	return s, nil
}

func apply(ctx context.Context, cl *kgo.Client, adm *kadm.Client, p catalog.Plan) error {
	for _, name := range slices.Sorted(maps.Keys(p.Create)) {
		t := p.Create[name]
		configs := map[string]*string{}
		for k, v := range t.Configs() {
			configs[k] = &v
		}
		r, err := adm.CreateTopic(ctx, t.Partitions, -1, configs, name)
		if err == nil {
			err = r.Err
		}
		if err != nil {
			return fmt.Errorf("create topic %s: %w", name, err)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(p.Extend)) {
		rs, err := adm.UpdatePartitions(ctx, p.Extend[name], name)
		if err == nil {
			err = rs.Error()
		}
		if err != nil {
			return fmt.Errorf("extend topic %s: %w", name, err)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(p.Configure)) {
		var alters []kadm.AlterConfig
		for k, v := range p.Configure[name] {
			alters = append(alters, kadm.AlterConfig{Op: kadm.SetConfig, Name: k, Value: &v})
		}
		rs, err := adm.AlterTopicConfigs(ctx, alters, name)
		for _, r := range rs {
			err = errors.Join(err, r.Err)
		}
		if err != nil {
			return fmt.Errorf("configure topic %s: %w", name, err)
		}
	}
	if err := addACLs(ctx, cl, p.Add); err != nil {
		return err
	}
	return removeACLs(ctx, cl, p.Remove)
}

type entry struct {
	resource   kmsg.ACLResourceType
	pattern    kmsg.ACLResourcePatternType
	operation  kmsg.ACLOperation
	permission kmsg.ACLPermissionType
}

func parse(a catalog.ACL) (entry, error) {
	var e entry
	var errs [4]error
	e.resource, errs[0] = kmsg.ParseACLResourceType(a.Resource)
	e.pattern, errs[1] = kmsg.ParseACLResourcePatternType(a.Pattern)
	e.operation, errs[2] = kmsg.ParseACLOperation(a.Operation)
	e.permission, errs[3] = kmsg.ParseACLPermissionType(a.Permission)
	return e, errors.Join(errs[:]...)
}

func addACLs(ctx context.Context, cl *kgo.Client, acls []catalog.ACL) error {
	if len(acls) == 0 {
		return nil
	}
	req := kmsg.NewPtrCreateACLsRequest()
	for _, a := range acls {
		e, err := parse(a)
		if err != nil {
			return fmt.Errorf("ACL %+v: %w", a, err)
		}
		c := kmsg.NewCreateACLsRequestCreation()
		c.ResourceType, c.ResourceName, c.ResourcePatternType = e.resource, a.Name, e.pattern
		c.Principal, c.Host, c.Operation, c.PermissionType = a.Principal, a.Host, e.operation, e.permission
		req.Creations = append(req.Creations, c)
	}
	resp, err := req.RequestWith(ctx, cl)
	if err != nil {
		return fmt.Errorf("create ACLs: %w", err)
	}
	for i, r := range resp.Results {
		if err := kerr.ErrorForCode(r.ErrorCode); err != nil {
			return fmt.Errorf("create ACL %+v: %w", acls[i], err)
		}
	}
	return nil
}

func removeACLs(ctx context.Context, cl *kgo.Client, acls []catalog.ACL) error {
	if len(acls) == 0 {
		return nil
	}
	req := kmsg.NewPtrDeleteACLsRequest()
	for _, a := range acls {
		e, err := parse(a)
		if err != nil {
			return fmt.Errorf("ACL %+v: %w", a, err)
		}
		f := kmsg.NewDeleteACLsRequestFilter()
		f.ResourceType, f.ResourceName, f.ResourcePatternType = e.resource, &a.Name, e.pattern
		f.Principal, f.Host, f.Operation, f.PermissionType = &a.Principal, &a.Host, e.operation, e.permission
		req.Filters = append(req.Filters, f)
	}
	resp, err := req.RequestWith(ctx, cl)
	if err != nil {
		return fmt.Errorf("delete ACLs: %w", err)
	}
	for i, r := range resp.Results {
		if err := kerr.ErrorForCode(r.ErrorCode); err != nil {
			return fmt.Errorf("delete ACL %+v: %w", acls[i], err)
		}
	}
	return nil
}
