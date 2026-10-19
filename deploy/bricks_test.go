package deploy_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type listener struct {
	Port           int  `yaml:"port"`
	TLS            bool `yaml:"tls"`
	Authentication struct {
		Type string `yaml:"type"`
	} `yaml:"authentication"`
}

type brick struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		Kafka struct {
			Listeners     []listener `yaml:"listeners"`
			Authorization struct {
				Type       string   `yaml:"type"`
				SuperUsers []string `yaml:"superUsers"`
			} `yaml:"authorization"`
		} `yaml:"kafka"`
		EntityOperator any `yaml:"entityOperator"`
	} `yaml:"spec"`
}

func checkBricks(dir string) error {
	var docs []brick
	for _, f := range []string{"postgres.yaml", "kafka.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return err
		}
		dec := yaml.NewDecoder(bytes.NewReader(data))
		for {
			var b brick
			if err := dec.Decode(&b); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return fmt.Errorf("%s: %w", f, err)
			}
			docs = append(docs, b)
		}
	}
	find := func(kind string) *brick {
		for i := range docs {
			if docs[i].Kind == kind {
				return &docs[i]
			}
		}
		return nil
	}
	var errs []error
	if pg := find("Cluster"); pg == nil || pg.Metadata.Name != "postgres" || pg.Metadata.Namespace != "postgres" {
		errs = append(errs, errors.New("no Cluster postgres in namespace postgres, which internal/netpol targets"))
	}
	k := find("Kafka")
	if k == nil || k.Metadata.Name != "kafka" || k.Metadata.Namespace != "kafka" {
		return errors.Join(append(errs, errors.New("no Kafka kafka in namespace kafka, which KAFKA_BROKERS of catalog-apply names"))...)
	}
	if !slices.ContainsFunc(k.Spec.Kafka.Listeners, func(l listener) bool {
		return l.Port == 9093 && l.TLS && l.Authentication.Type == "tls"
	}) {
		errs = append(errs, errors.New("kafka: no listener 9093 with TLS and tls authentication, which catalog-apply dials"))
	}
	if k.Spec.Kafka.Authorization.Type != "simple" || !slices.Contains(k.Spec.Kafka.Authorization.SuperUsers, "CN=catalog-apply") {
		errs = append(errs, errors.New("kafka: authorization is not simple with superUsers CN=catalog-apply"))
	}
	if k.Spec.EntityOperator != nil {
		errs = append(errs, errors.New("kafka: an entityOperator would be a second writer of topics and users"))
	}
	return errors.Join(errs...)
}

func TestTheBricksAreWhatTheJobsAndPoliciesAssume(t *testing.T) {
	if err := checkBricks("bricks"); err != nil {
		t.Fatal(err)
	}
}

func TestTheBrickGuardFails(t *testing.T) {
	for _, tc := range []struct{ name, file, old, new, want string }{
		{"postgres renamed", "postgres.yaml", "kind: Cluster\nmetadata:\n  name: postgres", "kind: Cluster\nmetadata:\n  name: pg", "no Cluster postgres"},
		{"postgres elsewhere", "postgres.yaml", "  name: postgres\n  namespace: postgres\nspec:\n  instances", "  name: postgres\n  namespace: db\nspec:\n  instances", "no Cluster postgres"},
		{"kafka renamed", "kafka.yaml", "kind: Kafka\nmetadata:\n  name: kafka", "kind: Kafka\nmetadata:\n  name: events", "no Kafka kafka"},
		{"listener on another port", "kafka.yaml", "port: 9093", "port: 9094", "no listener 9093"},
		{"listener without mTLS", "kafka.yaml", "authentication:\n          type: tls", "authentication:\n          type: scram-sha-512", "no listener 9093"},
		{"catalog-apply not super", "kafka.yaml", "- CN=catalog-apply", "- CN=admin", "superUsers CN=catalog-apply"},
		{"entity operator", "kafka.yaml", "  clientsCa:", "  entityOperator:\n    topicOperator: {}\n  clientsCa:", "entityOperator"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"postgres.yaml", "kafka.yaml"} {
				data, err := os.ReadFile(filepath.Join("bricks", f))
				if err != nil {
					t.Fatal(err)
				}
				if f == tc.file {
					if !bytes.Contains(data, []byte(tc.old)) {
						t.Fatalf("%s does not hold %q", f, tc.old)
					}
					data = bytes.Replace(data, []byte(tc.old), []byte(tc.new), 1)
				}
				if err := os.WriteFile(filepath.Join(dir, f), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := checkBricks(dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkBricks = %v, want an error with %q", err, tc.want)
			}
		})
	}
}
