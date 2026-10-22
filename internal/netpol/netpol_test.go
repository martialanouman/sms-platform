package netpol_test

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"reflect"
	"slices"
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
		Ingress     []struct {
			From  []peer `yaml:"from"`
			Ports []struct {
				Protocol string `yaml:"protocol"`
				Port     int    `yaml:"port"`
			} `yaml:"ports"`
		} `yaml:"ingress"`
		Egress []struct {
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
	var order []string
	dec := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var p policy
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			want := []string{"default-deny", "dns"}
			for _, name := range slices.Sorted(maps.Keys(got)) {
				if !slices.Contains(want, name) {
					want = append(want, name)
				}
			}
			if !slices.Equal(order, want) {
				t.Errorf("policies in order %v, want %v", order, want)
			}
			return got
		} else if err != nil {
			t.Fatal(err)
		}
		order = append(order, p.Metadata.Name)
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
		"migrate.yaml": {Data: []byte(job("migrate", [2]string{"LOG_LEVEL", "debug"}) + "---\n" + strings.Replace(job("migrate", [2]string{"POSTGRES_URL", "x"}), "containers:", "initContainers:", 1))},
		"apply.yaml":   {Data: []byte(job("apply", [2]string{"KAFKA_BROKERS", "kafka-kafka-bootstrap.kafka.svc:9093, events-kafka-bootstrap.stream.svc:9094, kafka-kafka-bootstrap.kafka.svc:9093,"}, [2]string{"POSTGRES_URL", "x"}, [2]string{"POSTGRES_URL", "x"}))},
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
	if got, want := flows("apply"), []flow{{"kafka", "strimzi.io/cluster=kafka", 9093}, {"stream", "strimzi.io/cluster=events", 9094}, {"postgres", "cnpg.io/cluster=postgres", 5432}}; !reflect.DeepEqual(got, want) {
		t.Errorf("apply flows %v, want %v", got, want)
	}
}

func TestRenderRefusesABrokerOutsideStrimzi(t *testing.T) {
	for _, broker := range []string{"kafka-kafka-bootstrap:9093", "kafka.example.com:9093", "kafka-kafka-bootstrap.kafka.svc", "kafka-kafka-bootstrap.kafka.svc:tls",
		"kafka-kafka-bootstrap.kafka.svc:9093/x", "evil.kafka-kafka-bootstrap.kafka.svc:9093",
		"kafka-kafka-bootstrap.kafka.svc:0", "kafka-kafka-bootstrap.kafka.svc:65536"} {
		_, err := netpol.Render(fstest.MapFS{"apply.yaml": {Data: []byte(job("apply", [2]string{"KAFKA_BROKERS", broker}))}})
		if err == nil || !strings.Contains(err.Error(), "apply") {
			t.Errorf("broker %q: err = %v, want a refusal naming apply", broker, err)
		}
	}
}

func TestRenderIsHeadedAndSorted(t *testing.T) {
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
}

func TestRenderRefusesAWorkloadNamedLikeAFixedPolicy(t *testing.T) {
	for _, name := range []string{"dns", "default-deny"} {
		_, err := netpol.Render(fstest.MapFS{name + ".yaml": {Data: []byte(job(name, [2]string{"POSTGRES_URL", "x"}))}})
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("workload %s: err = %v, want a refusal", name, err)
		}
	}
}

func TestAServerReachesOpenBaoAndAdmitsOnlyItsCallers(t *testing.T) {
	got := render(t, fstest.MapFS{
		"content-key-svc.yaml": {Data: []byte(job("content-key-svc",
			[2]string{"OPENBAO_ADDR", "https://bao.secrets.svc:8200"}, [2]string{"GRPC_ADDR", ":7002"},
			[2]string{"TLS_ALLOWED_CLIENTS", "admin-api-svc, router-svc,"}))},
		"listener.yaml": {Data: []byte(job("listener", [2]string{"GRPC_ADDR", ":7003"}, [2]string{"TLS_ALLOWED_CLIENTS", "dashboard"}))},
		"jwks.yaml": {Data: []byte(job("jwks", [2]string{"HTTP_ADDR", ":8080"}, [2]string{"INTERNAL_ADDR", ":8443"},
			[2]string{"TLS_ALLOWED_CLIENTS", "admin-api-svc"}))},
		"both.yaml": {Data: []byte(job("both", [2]string{"GRPC_ADDR", ":7004"}, [2]string{"INTERNAL_ADDR", ":8444"},
			[2]string{"TLS_ALLOWED_CLIENTS", "admin-api-svc"}))},
	})
	if l := got["listener"]; len(l.Spec.Egress) != 0 || len(l.Spec.Ingress) != 1 || l.Spec.Ingress[0].Ports[0].Port != 7003 {
		t.Errorf("listener without egress: %+v", l.Spec)
	}
	if j := got["jwks"].Spec.Ingress; len(j) != 1 || len(j[0].Ports) != 1 || j[0].Ports[0].Port != 8443 || len(j[0].From) != 1 ||
		j[0].From[0].PodSelector.MatchLabels["app.kubernetes.io/name"] != "admin-api-svc" {
		t.Errorf("internal listener: %+v", j)
	}
	if b := got["both"].Spec.Ingress; len(b) != 1 || len(b[0].Ports) != 2 || b[0].Ports[0].Port != 7004 || b[0].Ports[1].Port != 8444 {
		t.Errorf("both listeners: %+v", b)
	}
	p := got["content-key-svc"]
	if !reflect.DeepEqual(p.Spec.PolicyTypes, []string{"Ingress", "Egress"}) {
		t.Errorf("policy types %v", p.Spec.PolicyTypes)
	}
	if e := p.Spec.Egress; len(e) != 1 || len(e[0].To) != 1 ||
		!reflect.DeepEqual(e[0].To[0].NamespaceSelector.MatchLabels, map[string]string{"kubernetes.io/metadata.name": "secrets"}) ||
		!reflect.DeepEqual(e[0].To[0].PodSelector.MatchLabels, map[string]string{"app.kubernetes.io/name": "bao"}) ||
		len(e[0].Ports) != 1 || e[0].Ports[0].Protocol != "TCP" || e[0].Ports[0].Port != 8200 {
		t.Errorf("egress %+v, want OpenBao on 8200", e)
	}
	in := p.Spec.Ingress
	if len(in) != 1 || len(in[0].Ports) != 1 || in[0].Ports[0].Protocol != "TCP" || in[0].Ports[0].Port != 7002 {
		t.Fatalf("ingress %+v, want one rule on 7002", in)
	}
	var callers []string
	for _, f := range in[0].From {
		if !reflect.DeepEqual(f.NamespaceSelector.MatchLabels, map[string]string{"kubernetes.io/metadata.name": "sms"}) {
			t.Errorf("caller from namespace %v", f.NamespaceSelector.MatchLabels)
		}
		callers = append(callers, f.PodSelector.MatchLabels["app.kubernetes.io/name"])
	}
	if !slices.Equal(callers, []string{"admin-api-svc", "router-svc"}) {
		t.Errorf("callers %v", callers)
	}
}

func TestRenderRefusesAServerItCannotBound(t *testing.T) {
	for name, env := range map[string][][2]string{
		"OpenBao in clear":         {{"OPENBAO_ADDR", "http://openbao.openbao.svc:8200"}},
		"OpenBao outside cluster":  {{"OPENBAO_ADDR", "https://openbao.example.com:8200"}},
		"OpenBao without port":     {{"OPENBAO_ADDR", "https://openbao.openbao.svc"}},
		"OpenBao on port 0":        {{"OPENBAO_ADDR", "https://openbao.openbao.svc:0"}},
		"OpenBao with a path":      {{"OPENBAO_ADDR", "https://openbao.openbao.svc:8200/v1"}},
		"callers without port":     {{"TLS_ALLOWED_CLIENTS", "admin-api-svc"}},
		"port without callers":     {{"GRPC_ADDR", ":7002"}},
		"internal without callers": {{"INTERNAL_ADDR", ":8443"}},
		"internal with a host":     {{"INTERNAL_ADDR", "0.0.0.0:8443"}, {"TLS_ALLOWED_CLIENTS", "admin-api-svc"}},
		"port with a host":         {{"GRPC_ADDR", "0.0.0.0:7002"}, {"TLS_ALLOWED_CLIENTS", "admin-api-svc"}},
		"port beyond range":        {{"GRPC_ADDR", ":65536"}, {"TLS_ALLOWED_CLIENTS", "admin-api-svc"}},
		"caller not a pod name":    {{"GRPC_ADDR", ":7002"}, {"TLS_ALLOWED_CLIENTS", "Admin_API"}},
	} {
		_, err := netpol.Render(fstest.MapFS{"server.yaml": {Data: []byte(job("server", env...))}})
		if err == nil || !strings.Contains(err.Error(), "server") {
			t.Errorf("%s: err = %v, want a refusal naming server", name, err)
		}
	}
}

func TestPrometheusReachesTheOpsPortOnly(t *testing.T) {
	port := func(name, number string) string {
		return "            - name: " + name + "\n              containerPort: " + number + "\n"
	}
	ports := "          ports:\n" + port("grpc", "7002") + port("ops", "9191") + port("metrics", "9292")
	initOps := strings.Replace(job("unprobed"), "      containers:", "      initContainers:\n        - name: init\n          ports:\n"+port("ops", "9191")+"      containers:", 1)
	got := render(t, fstest.MapFS{
		"server.yaml":   {Data: []byte(job("server", [2]string{"GRPC_ADDR", ":7002"}, [2]string{"TLS_ALLOWED_CLIENTS", "dashboard"}) + ports)},
		"probed.yaml":   {Data: []byte(job("probed") + ports)},
		"unprobed.yaml": {Data: []byte(initOps + "          ports:\n" + port("metrics", "9292"))},
	})
	if p, ok := got["unprobed"]; ok {
		t.Errorf("unprobed = %+v, want no policy: neither metrics nor an init container's ops is the ops port", p.Spec)
	}
	for _, name := range []string{"server", "probed"} {
		p := got[name]
		if !reflect.DeepEqual(p.Spec.PolicyTypes, []string{"Ingress", "Egress"}) {
			t.Errorf("%s: policy types %v", name, p.Spec.PolicyTypes)
		}
		in := p.Spec.Ingress
		if len(in) == 0 {
			t.Fatalf("%s: no ingress", name)
		}
		ops := in[len(in)-1]
		if len(ops.From) != 1 || len(ops.Ports) != 1 || ops.Ports[0].Protocol != "TCP" || ops.Ports[0].Port != 9191 ||
			!reflect.DeepEqual(ops.From[0].NamespaceSelector.MatchLabels, map[string]string{"kubernetes.io/metadata.name": "monitoring"}) ||
			!reflect.DeepEqual(ops.From[0].PodSelector.MatchLabels, map[string]string{"app.kubernetes.io/name": "prometheus"}) {
			t.Errorf("%s: ops ingress %+v, want Prometheus on 9191", name, ops)
		}
	}
	if in := got["server"].Spec.Ingress; len(in) != 2 || in[0].Ports[0].Port != 7002 {
		t.Errorf("server ingress %+v, want its callers on 7002 then Prometheus", in)
	}
	if len(got["probed"].Spec.Ingress) != 1 || len(got["probed"].Spec.Egress) != 0 {
		t.Errorf("probed = %+v, want the ops ingress alone", got["probed"].Spec)
	}
}

func TestAServiceReachesTheRelayOfSMTPAddr(t *testing.T) {
	got := render(t, fstest.MapFS{"mailer.yaml": {Data: []byte(job("mailer", [2]string{"SMTP_ADDR", "mailpit.mail.svc:1025"}))}})
	e := got["mailer"].Spec.Egress
	if len(e) != 1 || len(e[0].To) != 1 ||
		!reflect.DeepEqual(e[0].To[0].NamespaceSelector.MatchLabels, map[string]string{"kubernetes.io/metadata.name": "mail"}) ||
		!reflect.DeepEqual(e[0].To[0].PodSelector.MatchLabels, map[string]string{"app.kubernetes.io/name": "mailpit"}) ||
		len(e[0].Ports) != 1 || e[0].Ports[0].Protocol != "TCP" || e[0].Ports[0].Port != 1025 {
		t.Fatalf("egress %+v, want the relay on 1025", e)
	}
	for _, bad := range []string{"mailpit:1025", "smtp.example.com:25", "mailpit.mail.svc", "mailpit.mail.svc:0"} {
		if _, err := netpol.Render(fstest.MapFS{"mailer.yaml": {Data: []byte(job("mailer", [2]string{"SMTP_ADDR", bad}))}}); err == nil || !strings.Contains(err.Error(), "SMTP_ADDR") {
			t.Errorf("%q: err = %v, want a refusal", bad, err)
		}
	}
}

func TestAServiceReachesThePodOfItsJWKS(t *testing.T) {
	got := render(t, fstest.MapFS{"verifier.yaml": {Data: []byte(job("verifier", [2]string{"JWKS_URL", "https://issuer:9443/.well-known/jwks.json"}))}})
	e := got["verifier"].Spec.Egress
	if len(e) != 1 || len(e[0].To) != 1 ||
		!reflect.DeepEqual(e[0].To[0].NamespaceSelector.MatchLabels, map[string]string{"kubernetes.io/metadata.name": "sms"}) ||
		!reflect.DeepEqual(e[0].To[0].PodSelector.MatchLabels, map[string]string{"app.kubernetes.io/name": "issuer"}) ||
		len(e[0].Ports) != 1 || e[0].Ports[0].Protocol != "TCP" || e[0].Ports[0].Port != 9443 {
		t.Fatalf("egress %+v, want the issuer's pods on 9443", e)
	}
	for _, bad := range []string{"http://issuer:8443/.well-known/jwks.json", "https://issuer/.well-known/jwks.json",
		"https://issuer.sms.svc:8443/jwks", "https://issuer:0/jwks", "https://Issuer:8443/jwks", "https://issuer:8443"} {
		if _, err := netpol.Render(fstest.MapFS{"verifier.yaml": {Data: []byte(job("verifier", [2]string{"JWKS_URL", bad}))}}); err == nil || !strings.Contains(err.Error(), "JWKS_URL") {
			t.Errorf("%q: err = %v, want a refusal", bad, err)
		}
	}
}

func TestAServiceReachesThePodsOfItsAdminAPI(t *testing.T) {
	got := render(t, fstest.MapFS{"relay.yaml": {Data: []byte(job("relay", [2]string{"ADMIN_API_URL", "https://gateway:9443"}))}})
	e := got["relay"].Spec.Egress
	if len(e) != 1 || len(e[0].To) != 1 ||
		!reflect.DeepEqual(e[0].To[0].NamespaceSelector.MatchLabels, map[string]string{"kubernetes.io/metadata.name": "sms"}) ||
		!reflect.DeepEqual(e[0].To[0].PodSelector.MatchLabels, map[string]string{"app.kubernetes.io/name": "gateway"}) ||
		len(e[0].Ports) != 1 || e[0].Ports[0].Port != 9443 {
		t.Fatalf("egress %+v, want the gateway's pods on 9443", e)
	}
	for _, bad := range []string{"http://gateway:9443", "https://gateway", "https://gateway.sms.svc:9443", "https://gateway:0",
		"https://Gateway:9443", "https://gateway:9443/admin"} {
		if _, err := netpol.Render(fstest.MapFS{"relay.yaml": {Data: []byte(job("relay", [2]string{"ADMIN_API_URL", bad}))}}); err == nil || !strings.Contains(err.Error(), "ADMIN_API_URL") {
			t.Errorf("%q: err = %v, want a refusal", bad, err)
		}
	}
}
