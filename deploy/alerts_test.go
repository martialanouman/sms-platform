package deploy_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func checkAlerts(dir string) error {
	rules, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return err
	}
	var errs []error
	for _, file := range rules {
		base := strings.TrimSuffix(file, ".yaml")
		if strings.HasSuffix(base, "_test") {
			continue
		}
		var r struct {
			Groups []struct {
				Rules []struct {
					Alert string `yaml:"alert"`
				} `yaml:"rules"`
			} `yaml:"groups"`
		}
		var tests struct {
			Tests []struct {
				AlertRuleTest []struct {
					Alertname string `yaml:"alertname"`
					ExpAlerts []any  `yaml:"exp_alerts"`
				} `yaml:"alert_rule_test"`
			} `yaml:"tests"`
		}
		for _, f := range []struct {
			path string
			into any
		}{{file, &r}, {base + "_test.yaml", &tests}} {
			data, err := os.ReadFile(f.path)
			if err == nil {
				err = yaml.Unmarshal(data, f.into)
			}
			if err != nil {
				errs = append(errs, err)
			}
		}
		fires, silent := map[string]bool{}, map[string]bool{}
		for _, t := range tests.Tests {
			for _, c := range t.AlertRuleTest {
				if len(c.ExpAlerts) > 0 {
					fires[c.Alertname] = true
				} else {
					silent[c.Alertname] = true
				}
			}
		}
		for _, g := range r.Groups {
			for _, rule := range g.Rules {
				if rule.Alert != "" && (!fires[rule.Alert] || !silent[rule.Alert]) {
					errs = append(errs, fmt.Errorf("%s: alert %s fires in a test %v, stays silent in one %v; want both",
						filepath.Base(file), rule.Alert, fires[rule.Alert], silent[rule.Alert]))
				}
			}
		}
	}
	return errors.Join(errs...)
}

func TestEachAlertFiresAndStaysSilentInATest(t *testing.T) {
	if err := checkAlerts("alerts"); err != nil {
		t.Fatal(err)
	}
}

func TestTheAlertGuardFails(t *testing.T) {
	for _, tc := range []struct{ name, file, old, new, want string }{
		{"no firing case", "ops_test.yaml", "exp_alerts:\n          - exp_labels:", "exp_alerts: []\n        x:\n          - exp_labels:", "fires in a test false"},
		{"no silent case", "ops_test.yaml", "exp_alerts: []", "exp_alerts: [{}]", "stays silent in one false"},
		{"alert renamed", "ops.yaml", "alert: MetricsGatheredPartially", "alert: MetricsServedPartially", "alert MetricsServedPartially"},
		{"no test file", "ops_test.yaml", "", "", "ops_test.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"ops.yaml", "ops_test.yaml"} {
				data, err := os.ReadFile(filepath.Join("alerts", f))
				if err != nil {
					t.Fatal(err)
				}
				if f == tc.file {
					if tc.old == "" {
						continue
					}
					if !bytes.Contains(data, []byte(tc.old)) {
						t.Fatalf("%s does not hold %q", f, tc.old)
					}
					data = bytes.ReplaceAll(data, []byte(tc.old), []byte(tc.new))
				}
				if err := os.WriteFile(filepath.Join(dir, f), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := checkAlerts(dir); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("checkAlerts = %v, want an error with %q", err, tc.want)
			}
		})
	}
}
