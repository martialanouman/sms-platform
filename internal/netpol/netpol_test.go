package netpol_test

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"go.yaml.in/yaml/v3"

	"github.com/martialanouman/sms-platform/internal/netpol"
)

type peer struct {
	NamespaceSelector struct {
		MatchLabels map[string]string `yaml:"matchLabels"`
	} `yaml:"namespaceSelector"`
	PodSelector struct {
		MatchLabels map[string]string `yaml:"matchLabels"`
	} `yaml:"podSelector"`
}

type policy struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		PodSelector struct {
			MatchLabels map[string]string `yaml:"matchLabels"`
		} `yaml:"podSelector"`
		PolicyTypes []string `yaml:"policyTypes"`
		Ingress     []any    `yaml:"ingress"`
		Egress      []struct {
			To    []peer `yaml:"to"`
			Ports []struct {
				Protocol string `yaml:"protocol"`
				Port     int    `yaml:"port"`
			} `yaml:"ports"`
		} `yaml:"egress"`
	} `yaml:"spec"`
}

func job(name string, env ...[2]string) string {
	var b strings.Builder
	b.WriteString("apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: " + name + "\nspec:\n  template:\n    spec:\n      containers:\n        - name: " + name + "\n          env:\n")
	for _, e := range env {
		b.WriteString("            - name: " + e[0] + "\n              value: " + e[1] + "\n")
	}
	return b.String()
}

func render(t *testing.T, files fstest.MapFS) map[string]policy {
	t.Helper()
	out, err := netpol.Render(files)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]policy{}
	dec := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var p policy
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			return got
		} else if err != nil {
			t.Fatal(err)
		}
		if p.APIVersion != "networking.k8s.io/v1" || p.Kind != "NetworkPolicy" || p.Metadata.Namespace != "" {
			t.Errorf("%s: %s %s in namespace %q", p.Metadata.Name, p.APIVersion, p.Kind, p.Metadata.Namespace)
		}
		got[p.Metadata.Name] = p
	}
}

func TestEveryPodIsDeniedThenResolvesNames(t *testing.T) {
	got := render(t, fstest.MapFS{"idle.yaml": {Data: []byte(job("idle"))}, netpol.File: {Data: []byte("garbage: [")}})
	if len(got) != 2 {
		t.Errorf("policies %v, want default-deny and dns only", reflect.ValueOf(got).MapKeys())
	}
	deny := got["default-deny"]
	if deny.Spec.PodSelector.MatchLabels != nil || !reflect.DeepEqual(deny.Spec.PolicyTypes, []string{"Ingress", "Egress"}) ||
		deny.Spec.Ingress != nil || deny.Spec.Egress != nil {
		t.Errorf("default-deny = %+v", deny.Spec)
	}
	dns := got["dns"]
	if dns.Spec.PodSelector.MatchLabels != nil || !reflect.DeepEqual(dns.Spec.PolicyTypes, []string{"Egress"}) || len(dns.Spec.Egress) != 1 {
		t.Fatalf("dns = %+v", dns.Spec)
	}
	e := dns.Spec.Egress[0]
	if len(e.To) != 1 || !reflect.DeepEqual(e.To[0].NamespaceSelector.MatchLabels, map[string]string{"kubernetes.io/metadata.name": "kube-system"}) ||
		!reflect.DeepEqual(e.To[0].PodSelector.MatchLabels, map[string]string{"k8s-app": "kube-dns"}) ||
		len(e.Ports) != 2 || e.Ports[0].Protocol+e.Ports[1].Protocol != "UDPTCP" || e.Ports[0].Port != 53 || e.Ports[1].Port != 53 {
		t.Errorf("dns egress = %+v", e)
	}
}

func TestEachWorkloadReachesTheStoresItsVariablesName(t *testing.T) {
	got := render(t, fstest.MapFS{
		"migrate.yaml": {Data: []byte(job("migrate", [2]string{"POSTGRES_URL", "x"}, [2]string{"LOG_LEVEL", "debug"}))},
		"apply.yaml":   {Data: []byte(job("apply", [2]string{"KAFKA_BROKERS", "kafka-kafka-bootstrap.kafka.svc:9093, events-kafka-bootstrap.stream.svc:9094, kafka-kafka-bootstrap.kafka.svc:9093"}))},
	})
	type flow struct {
		namespace, pod string
		port           int
	}
	flows := func(name string) []flow {
		p, ok := got[name]
		if !ok {
			t.Fatalf("no policy for %s", name)
		}
		if !reflect.DeepEqual(p.Spec.PodSelector.MatchLabels, map[string]string{"app.kubernetes.io/name": name}) ||
			!reflect.DeepEqual(p.Spec.PolicyTypes, []string{"Egress"}) {
			t.Errorf("%s selects %v for %v", name, p.Spec.PodSelector.MatchLabels, p.Spec.PolicyTypes)
		}
		var fs []flow
		for _, e := range p.Spec.Egress {
			if len(e.To) != 1 || len(e.Ports) != 1 || e.Ports[0].Protocol != "TCP" {
				t.Errorf("%s: egress %+v", name, e)
				continue
			}
			var pod []string
			for k, v := range e.To[0].PodSelector.MatchLabels {
				pod = append(pod, k+"="+v)
			}
			fs = append(fs, flow{e.To[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"], strings.Join(pod, ","), e.Ports[0].Port})
		}
		return fs
	}
	if got, want := flows("migrate"), []flow{{"postgres", "cnpg.io/cluster=postgres", 5432}}; !reflect.DeepEqual(got, want) {
		t.Errorf("migrate flows %v, want %v", got, want)
	}
	if got, want := flows("apply"), []flow{{"kafka", "strimzi.io/cluster=kafka", 9093}, {"stream", "strimzi.io/cluster=events", 9094}}; !reflect.DeepEqual(got, want) {
		t.Errorf("apply flows %v, want %v", got, want)
	}
}

func TestRenderRefusesABrokerOutsideStrimzi(t *testing.T) {
	for _, broker := range []string{"kafka-kafka-bootstrap:9093", "kafka.example.com:9093", "kafka-kafka-bootstrap.kafka.svc", "kafka-kafka-bootstrap.kafka.svc:tls",
		"kafka-kafka-bootstrap.kafka.svc:9093/x", "evil.kafka-kafka-bootstrap.kafka.svc:9093"} {
		_, err := netpol.Render(fstest.MapFS{"apply.yaml": {Data: []byte(job("apply", [2]string{"KAFKA_BROKERS", broker}))}})
		if err == nil || !strings.Contains(err.Error(), "apply") {
			t.Errorf("broker %q: err = %v, want a refusal naming apply", broker, err)
		}
	}
}

func TestRenderIsStable(t *testing.T) {
	files := fstest.MapFS{
		"b.yaml": {Data: []byte(job("b", [2]string{"POSTGRES_URL", "x"}))},
		"a.yaml": {Data: []byte(job("a", [2]string{"POSTGRES_URL", "x"}))},
	}
	first, err := netpol.Render(files)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(first), "# Code generated") || strings.Index(string(first), "name: a\n") > strings.Index(string(first), "name: b\n") {
		t.Errorf("render is not headed and sorted:\n%s", first)
	}
	for range 20 {
		if again, _ := netpol.Render(files); !bytes.Equal(again, first) {
			t.Fatal("two renderings differ")
		}
	}
}
