package deploy_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/martialanouman/sms-platform/contracts"
	"github.com/martialanouman/sms-platform/internal/catalog"
	contentkeygrpc "github.com/martialanouman/sms-platform/internal/contentkey/grpc"
	"github.com/martialanouman/sms-platform/internal/netpol"
	"github.com/martialanouman/sms-platform/internal/platform/config"
	"github.com/martialanouman/sms-platform/internal/schema"
)

const registry = "ghcr.io/martialanouman/sms-platform/"

var (
	deployedJobs      = regexp.MustCompile(`(?m)^for job in ([a-z -]+); do$`)
	deployedServices  = regexp.MustCompile(`(?m)^for service in ([a-z -]+); do$`)
	awaited           = regexp.MustCompile(`(?m)^\tkubectl -n sms rollout status "deploy/\$service" --timeout=[0-9]+m$`)
	rulesCreated      = regexp.MustCompile(`(?m)^kubectl -n monitoring create configmap alert-rules `)
	monitoringBrick   = regexp.MustCompile(`(?m)^kubectl apply -f deploy/bricks/monitoring\.yaml$`)
	prometheusAwaited = regexp.MustCompile(`(?m)^kubectl -n monitoring rollout status deploy/prometheus-server --timeout=[0-9]+m$`)
)

type goreleaser struct {
	Builds []struct {
		ID   string   `yaml:"id"`
		Main string   `yaml:"main"`
		Env  []string `yaml:"env"`
	} `yaml:"builds"`
	Dockers []struct {
		ID     string   `yaml:"id"`
		IDs    []string `yaml:"ids"`
		Images []string `yaml:"images"`
	} `yaml:"dockers_v2"`
}

type container struct {
	Name            string `yaml:"name"`
	Image           string `yaml:"image"`
	SecurityContext struct {
		RunAsNonRoot           *bool `yaml:"runAsNonRoot"`
		ReadOnlyRootFilesystem *bool `yaml:"readOnlyRootFilesystem"`
		SeccompProfile         struct {
			Type string `yaml:"type"`
		} `yaml:"seccompProfile"`
	} `yaml:"securityContext"`
	ReadinessProbe struct {
		HTTPGet struct {
			Path string `yaml:"path"`
		} `yaml:"httpGet"`
	} `yaml:"readinessProbe"`
	Env []struct {
		Name      string `yaml:"name"`
		Value     string `yaml:"value"`
		ValueFrom struct {
			SecretKeyRef *struct{} `yaml:"secretKeyRef"`
		} `yaml:"valueFrom"`
	} `yaml:"env"`
	EnvFrom []any `yaml:"envFrom"`
}

type workload struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Type     string         `yaml:"type"`
		Selector map[string]any `yaml:"selector"`
		Ports    []struct {
			Port       int `yaml:"port"`
			TargetPort any `yaml:"targetPort"`
		} `yaml:"ports"`
		MaxReplicas    int32 `yaml:"maxReplicas"`
		ScaleTargetRef struct {
			Name string `yaml:"name"`
		} `yaml:"scaleTargetRef"`
		ActiveDeadlineSeconds int `yaml:"activeDeadlineSeconds"`
		Template              struct {
			Metadata struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"metadata"`
			Spec struct {
				AutomountServiceAccountToken  *bool       `yaml:"automountServiceAccountToken"`
				TerminationGracePeriodSeconds int         `yaml:"terminationGracePeriodSeconds"`
				Containers                    []container `yaml:"containers"`
				InitContainers                []container `yaml:"initContainers"`
				Volumes                       []struct {
					Projected struct {
						Sources []struct {
							Secret struct {
								Name  string `yaml:"name"`
								Items []struct {
									Key  string `yaml:"key"`
									Path string `yaml:"path"`
								} `yaml:"items"`
							} `yaml:"secret"`
						} `yaml:"sources"`
					} `yaml:"projected"`
				} `yaml:"volumes"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func check(root string) error {
	var errs []error
	entries, err := os.ReadDir(filepath.Join(root, "cmd"))
	if err != nil {
		return err
	}
	var cmds []string
	for _, e := range entries {
		if e.IsDir() {
			cmds = append(cmds, e.Name())
		}
	}
	data, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml"))
	if err != nil {
		return err
	}
	var g goreleaser
	if err := yaml.Unmarshal(data, &g); err != nil {
		return err
	}
	var builds, images []string
	for _, b := range g.Builds {
		builds = append(builds, b.ID)
		if b.Main != "./cmd/"+b.ID {
			errs = append(errs, fmt.Errorf("build %s: main is %q, want ./cmd/%s", b.ID, b.Main, b.ID))
		}
		if !slices.Contains(b.Env, "CGO_ENABLED=0") {
			errs = append(errs, fmt.Errorf("build %s: no CGO_ENABLED=0, the static image cannot run it", b.ID))
		}
	}
	for _, d := range g.Dockers {
		images = append(images, d.ID)
		if !slices.Equal(d.IDs, []string{d.ID}) || !slices.Equal(d.Images, []string{registry + d.ID}) {
			errs = append(errs, fmt.Errorf("image %s: ids %v and images %v, want [%s] and [%s%s]", d.ID, d.IDs, d.Images, d.ID, registry, d.ID))
		}
	}
	files, err := os.ReadDir(filepath.Join(root, "deploy", "k8s"))
	if err != nil {
		return err
	}
	var manifests []string
	kinds := map[string]string{}
	for _, f := range files {
		name, ok := strings.CutSuffix(f.Name(), ".yaml")
		if !ok {
			errs = append(errs, fmt.Errorf("deploy/k8s/%s is not a .yaml", f.Name()))
			continue
		}
		if f.Name() == netpol.File {
			errs = append(errs, checkPolicies(filepath.Join(root, "deploy", "k8s", f.Name()))...)
			continue
		}
		manifests = append(manifests, name)
		kind, merrs := checkManifest(filepath.Join(root, "deploy", "k8s", f.Name()), name)
		kinds[name] = kind
		errs = append(errs, merrs...)
	}
	script, err := os.ReadFile(filepath.Join(root, "deploy", "test-env", "deploy.sh"))
	if err != nil {
		return err
	}
	rules, brick, prometheus := rulesCreated.FindIndex(script), monitoringBrick.FindIndex(script), prometheusAwaited.FindIndex(script)
	if rules == nil || brick == nil || brick[0] < rules[0] {
		errs = append(errs, errors.New("deploy.sh: the ConfigMap alert-rules is not created before the brick monitoring, which mounts it"))
	}
	if prometheus == nil {
		errs = append(errs, errors.New("deploy.sh: prometheus-server is not awaited by rollout status"))
	}
	var jobs, services []string
	jobsAt, servicesAt := deployedJobs.FindSubmatchIndex(script), deployedServices.FindSubmatchIndex(script)
	if jobsAt != nil {
		jobs = strings.Fields(string(script[jobsAt[2]:jobsAt[3]]))
	}
	if servicesAt != nil {
		services = strings.Fields(string(script[servicesAt[2]:servicesAt[3]]))
		if jobsAt == nil || servicesAt[0] < jobsAt[0] {
			errs = append(errs, errors.New("deploy.sh: the services before the Jobs"))
		}
		loop, _, closed := bytes.Cut(script[servicesAt[1]:], []byte("\ndone\n"))
		if !closed || !awaited.Match(loop) {
			errs = append(errs, errors.New("deploy.sh: the services are not awaited by rollout status"))
		}
	}
	for _, j := range jobs {
		if k := kinds[j]; k != "" && k != "Job" {
			errs = append(errs, fmt.Errorf("deploy.sh: %s: a %s in the loop of the Jobs", j, k))
		}
	}
	for _, s := range services {
		if k := kinds[s]; k != "" && k != "Deployment" {
			errs = append(errs, fmt.Errorf("deploy.sh: %s: a %s in the loop of the services", s, k))
		}
	}
	errs = append(errs, roleSecrets(root)...)
	errs = append(errs, same("build", cmds, builds)...)
	errs = append(errs, same("image", cmds, images)...)
	errs = append(errs, same("manifest", cmds, manifests)...)
	errs = append(errs, same("deploy.sh job or service", cmds, append(jobs, services...))...)
	return errors.Join(errs...)
}

func same(what string, cmds, got []string) []error {
	var errs []error
	for _, c := range cmds {
		n := 0
		for _, g := range got {
			if g == c {
				n++
			}
		}
		if n != 1 {
			errs = append(errs, fmt.Errorf("cmd/%s: %d %s, want 1", c, n, what))
		}
	}
	for _, g := range got {
		if !slices.Contains(cmds, g) {
			errs = append(errs, fmt.Errorf("%s %s: no cmd/%s", what, g, g))
		}
	}
	return errs
}

func roleSecrets(root string) []error {
	grants, err := filepath.Glob(filepath.Join(root, "contracts", "grants", "*.yaml"))
	if err != nil {
		return []error{err}
	}
	var want, got []string
	for _, g := range grants {
		service := strings.TrimSuffix(filepath.Base(g), ".yaml")
		want = append(want, service+" postgres-password "+schema.Role(service))
	}
	data, err := os.ReadFile(filepath.Join(root, "deploy", "k8s", "catalog-apply.yaml"))
	if err != nil {
		return []error{err}
	}
	var w workload
	if err := yaml.Unmarshal(data, &w); err != nil {
		return []error{err}
	}
	for _, v := range w.Spec.Template.Spec.Volumes {
		for _, src := range v.Projected.Sources {
			for _, item := range src.Secret.Items {
				got = append(got, src.Secret.Name+" "+item.Key+" "+item.Path)
			}
		}
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		return []error{fmt.Errorf("catalog-apply role secrets %q, want one per contracts/grants declaration %q", got, want)}
	}
	return nil
}

func checkPolicies(path string) []error {
	data, err := os.ReadFile(path)
	if err != nil {
		return []error{err}
	}
	var errs []error
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc struct {
			Kind string `yaml:"kind"`
		}
		if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
			return errs
		} else if err != nil {
			return append(errs, fmt.Errorf("%s: %w", netpol.File, err))
		}
		if doc.Kind != "NetworkPolicy" {
			errs = append(errs, fmt.Errorf("%s: kind %q, want NetworkPolicy", netpol.File, doc.Kind))
		}
	}
}

func checkManifest(path, name string) (string, []error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", []error{err}
	}
	kind := ""
	var errs []error
	if !regexp.MustCompile(`(?m)^ *image: ` + regexp.QuoteMeta(registry+name) + `$`).Match(data) {
		errs = append(errs, fmt.Errorf("%s: image not in the form deploy.sh tags, %q", name, "image: "+registry+name))
	}
	grpcPort, servicePorts := 0, []int{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for docs := 0; ; docs++ {
		var w workload
		if err := dec.Decode(&w); errors.Is(err, io.EOF) {
			if docs == 0 {
				errs = append(errs, fmt.Errorf("%s holds no workload", name))
			}
			break
		} else if err != nil {
			return kind, append(errs, fmt.Errorf("%s: %w", path, err))
		}
		if w.Kind == "HorizontalPodAutoscaler" {
			c, err := catalog.Parse(contracts.Topics())
			if err != nil {
				return kind, append(errs, err)
			}
			if bound, by := partitionsBound(c, w.Spec.ScaleTargetRef.Name); w.Spec.MaxReplicas > bound {
				errs = append(errs, fmt.Errorf("%s: maxReplicas %d above the %d partitions of %s", name, w.Spec.MaxReplicas, bound, by))
			}
			continue
		}
		switch w.Kind {
		case "ServiceAccount", "Service":
			if w.Metadata.Name != name {
				errs = append(errs, fmt.Errorf("%s: %s %q, want %q", name, w.Kind, w.Metadata.Name, name))
			}
			if w.Kind == "Service" {
				if w.Spec.Type != "" && w.Spec.Type != "ClusterIP" {
					errs = append(errs, fmt.Errorf("%s: Service type %s, want ClusterIP", name, w.Spec.Type))
				}
				if !reflect.DeepEqual(w.Spec.Selector, map[string]any{"app.kubernetes.io/name": name}) {
					errs = append(errs, fmt.Errorf("%s: Service selector %v, want the pods of %s", name, w.Spec.Selector, name))
				}
			}
			for _, p := range w.Spec.Ports {
				servicePorts = append(servicePorts, p.Port)
				if p.TargetPort != nil {
					errs = append(errs, fmt.Errorf("%s: Service targetPort %v, want none: the port is the one of GRPC_ADDR", name, p.TargetPort))
				}
			}
			continue
		case "Job":
			if w.Spec.ActiveDeadlineSeconds <= 0 {
				errs = append(errs, fmt.Errorf("%s: no activeDeadlineSeconds, a stuck Job would hold the deployment", name))
			}
		case "Deployment":
			errs = append(errs, checkService(name, w)...)
		default:
			errs = append(errs, fmt.Errorf("%s: kind %q is not guarded", name, w.Kind))
			continue
		}
		kind = w.Kind
		if got := w.Spec.Template.Metadata.Labels["app.kubernetes.io/name"]; got != name {
			errs = append(errs, fmt.Errorf("%s: pod label app.kubernetes.io/name %q, want %q", name, got, name))
		}
		pod := w.Spec.Template.Spec
		if a := pod.AutomountServiceAccountToken; a == nil || *a {
			errs = append(errs, fmt.Errorf("%s: %s mounts the service account token, want automountServiceAccountToken false", name, w.Kind))
		}
		if len(pod.Containers) == 0 {
			errs = append(errs, fmt.Errorf("%s: %s without container", name, w.Kind))
		}
		for _, c := range append(pod.InitContainers, pod.Containers...) {
			at := fmt.Sprintf("%s: %s container %s", name, w.Kind, c.Name)
			if c.Image != registry+name {
				errs = append(errs, fmt.Errorf("%s: image %q, want %q", at, c.Image, registry+name))
			}
			if sc := c.SecurityContext; sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
				errs = append(errs, fmt.Errorf("%s: runAsNonRoot is not true", at))
			}
			if sc := c.SecurityContext; sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				errs = append(errs, fmt.Errorf("%s: readOnlyRootFilesystem is not true", at))
			}
			if c.SecurityContext.SeccompProfile.Type != "RuntimeDefault" {
				errs = append(errs, fmt.Errorf("%s: seccompProfile is not RuntimeDefault", at))
			}
			if len(c.EnvFrom) > 0 {
				errs = append(errs, fmt.Errorf("%s: envFrom hides the variables it sets", at))
			}
			for _, e := range c.Env {
				if !slices.Contains(config.Variables(), e.Name) {
					errs = append(errs, fmt.Errorf("%s: %s is not read by config", at, e.Name))
				}
				if e.Name == "POSTGRES_URL" && e.ValueFrom.SecretKeyRef == nil {
					errs = append(errs, fmt.Errorf("%s: POSTGRES_URL does not come from a secretKeyRef", at))
				}
				if port, ok := strings.CutPrefix(e.Value, ":"); e.Name == "GRPC_ADDR" && ok {
					grpcPort, _ = strconv.Atoi(port)
				}
			}
		}
	}
	for _, p := range servicePorts {
		if p != grpcPort {
			errs = append(errs, fmt.Errorf("%s: Service port %d, want the port of GRPC_ADDR %d", name, p, grpcPort))
		}
	}
	return kind, errs
}

func checkService(name string, w workload) []error {
	var errs []error
	for _, c := range w.Spec.Template.Spec.Containers {
		drain := time.Duration(0)
		for _, v := range []string{"DRAIN_DELAY", "DRAIN_BUDGET", "SHUTDOWN_TIMEOUT"} {
			value := config.Default(v)
			for _, e := range c.Env {
				if e.Name == v {
					value = e.Value
				}
			}
			d, err := time.ParseDuration(value)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %s: %w", name, v, err))
			}
			drain += d
		}
		if grace := time.Duration(w.Spec.Template.Spec.TerminationGracePeriodSeconds) * time.Second; grace < drain {
			errs = append(errs, fmt.Errorf("%s: container %s: terminationGracePeriodSeconds %s below its drain %s", name, c.Name, grace, drain))
		}
		if c.ReadinessProbe.HTTPGet.Path != "/readyz" {
			errs = append(errs, fmt.Errorf("%s: container %s: readinessProbe on %q, want /readyz", name, c.Name, c.ReadinessProbe.HTTPGet.Path))
		}
		for _, e := range c.Env {
			if e.Name != "TLS_ALLOWED_CLIENTS" || name != "content-key-svc" {
				continue
			}
			var allowed []string
			for a := range strings.SplitSeq(e.Value, ",") {
				if a = strings.TrimSpace(a); a != "" {
					allowed = append(allowed, a)
				}
			}
			if want := slices.Sorted(maps.Keys(contentkeygrpc.Callers)); !slices.Equal(slices.Sorted(slices.Values(allowed)), want) {
				errs = append(errs, fmt.Errorf("%s: TLS_ALLOWED_CLIENTS %v, want the callers of the method map %v", name, allowed, want))
			}
		}
	}
	return errs
}

// partitionsBound is the partition count of the smallest topic service consumes by group (spec 09 §9.1, guard 6):
// replicas beyond it sit idle.
func partitionsBound(c catalog.Catalog, service string) (int32, string) {
	bound, by := int32(math.MaxInt32), ""
	for _, g := range c.Groups {
		if p := c.Topics[g.Topic].Partitions; g.Service == service && g.Assignment != "broadcast" && g.Assignment != "manual" && p < bound {
			bound, by = p, g.Topic
		}
	}
	return bound, by
}

func TestTheTreeHoldsTheDeliveryRules(t *testing.T) {
	if err := check(".."); err != nil {
		t.Fatal(err)
	}
}

func TestEachDeliveryRuleFails(t *testing.T) {
	const initContainer = "      initContainers:\n        - name: init\n          image: ghcr.io/martialanouman/sms-platform/migrate\n" +
		"          securityContext:\n            readOnlyRootFilesystem: true\n            seccompProfile: {type: RuntimeDefault}\n      containers:\n"
	for _, tc := range []struct {
		name, file, old, new, want string
	}{
		{"cmd without build", ".goreleaser.yaml", "id: migrate\n    main:", "id: other\n    main:", "cmd/migrate: 0 build"},
		{"build without cmd", ".goreleaser.yaml", "id: migrate\n    main:", "id: other\n    main:", "build other: no cmd/other"},
		{"duplicate build", ".goreleaser.yaml", "builds:\n", "builds:\n  - id: migrate\n    main: ./cmd/migrate\n    env: [CGO_ENABLED=0]\n", "cmd/migrate: 2 build"},
		{"build of another main", ".goreleaser.yaml", "main: ./cmd/migrate", "main: ./cmd/other", "main is"},
		{"build with cgo", ".goreleaser.yaml", "env: [CGO_ENABLED=0]", "env: []", "CGO_ENABLED=0"},
		{"cmd without image", ".goreleaser.yaml", "dockers_v2:\n  - id: migrate", "dockers_v2:\n  - id: other", "cmd/migrate: 0 image"},
		{"image of another build", ".goreleaser.yaml", "ids: [migrate]", "ids: [other]", "image migrate: ids"},
		{"image in another repository", ".goreleaser.yaml", "images: [ghcr.io/martialanouman/sms-platform/migrate]", "images: [ghcr.io/other/migrate]", "image migrate: ids"},
		{"cmd without manifest", "deploy/k8s/migrate.yaml", "", "", "cmd/migrate: 0 manifest"},
		{"empty manifest", "deploy/k8s/migrate.yaml", "", "\n", "migrate holds no workload"},
		{"manifest of another extension", "deploy/k8s/other.yml", "", "kind: Job\n", "other.yml is not a .yaml"},
		{"kind not guarded", "deploy/k8s/migrate.yaml", "kind: Job", "kind: StatefulSet", "kind \"StatefulSet\""},
		{"workload without container", "deploy/k8s/migrate.yaml", "      containers:", "      container:", "Job without container"},
		{"wrong image in manifest", "deploy/k8s/migrate.yaml", "image: ghcr.io/martialanouman/sms-platform/migrate", "image: ghcr.io/martialanouman/sms-platform/migrate:latest", "image \""},
		{"root container", "deploy/k8s/migrate.yaml", "runAsNonRoot: true", "runAsNonRoot: false", "runAsNonRoot"},
		{"root init container", "deploy/k8s/migrate.yaml", "      containers:\n", initContainer, "container init: runAsNonRoot"},
		{"writable root filesystem", "deploy/k8s/migrate.yaml", "readOnlyRootFilesystem: true", "readOnlyRootFilesystem: false", "readOnlyRootFilesystem"},
		{"no seccomp profile", "deploy/k8s/migrate.yaml", "type: RuntimeDefault", "type: Unconfined", "seccompProfile"},
		{"variable config does not read", "deploy/k8s/migrate.yaml", "- name: POSTGRES_URL", "- name: DATABASE_URL", "DATABASE_URL is not read by config"},
		{"literal URL", "deploy/k8s/migrate.yaml", "valueFrom:\n                secretKeyRef:\n                  name: migrate\n                  key: postgres-url", "value: postgres://x", "secretKeyRef"},
		{"declaration without role secret", "contracts/grants/billing-svc.yaml", "", "control_plane: {}\n", "billing-svc postgres-password billing_svc"},
		{"role secret without declaration", "contracts/grants/admin-api-svc.yaml", "", "", "declaration []"},
		{"role secret under another key", "deploy/k8s/catalog-apply.yaml", "key: postgres-password", "key: password", "admin-api-svc password admin_api_svc"},
		{"role secret under another path", "deploy/k8s/catalog-apply.yaml", "path: admin_api_svc", "path: admin-api-svc", "admin-api-svc postgres-password admin-api-svc"},
		{"Job among the network policies", "deploy/k8s/networkpolicies.yaml", "", "kind: Job\n", "networkpolicies.yaml: kind \"Job\""},
		{"Job behind a network policy", "deploy/k8s/networkpolicies.yaml", "DO NOT EDIT.\n", "DO NOT EDIT.\nkind: NetworkPolicy\n---\nkind: Job\n---\n", "networkpolicies.yaml: kind \"Job\""},
		{"pod without its label", "deploy/k8s/migrate.yaml", "app.kubernetes.io/name: migrate", "app.kubernetes.io/part-of: migrate", "label app.kubernetes.io/name"},
		{"pod labelled for another", "deploy/k8s/migrate.yaml", "app.kubernetes.io/name: migrate", "app.kubernetes.io/name: other", "label app.kubernetes.io/name"},
		{"unbounded Job", "deploy/k8s/migrate.yaml", "  activeDeadlineSeconds: 300\n", "", "activeDeadlineSeconds"},
		{"negative Job deadline", "deploy/k8s/migrate.yaml", "activeDeadlineSeconds: 300", "activeDeadlineSeconds: -1", "activeDeadlineSeconds"},
		{"image in quotes", "deploy/k8s/migrate.yaml", "image: ghcr.io/martialanouman/sms-platform/migrate\n", "image: \"ghcr.io/martialanouman/sms-platform/migrate\"\n", "not in the form deploy.sh tags"},
		{"image behind a comment", "deploy/k8s/migrate.yaml", "image: ghcr.io/martialanouman/sms-platform/migrate\n", "image: ghcr.io/martialanouman/sms-platform/migrate # pinned\n", "not in the form deploy.sh tags"},
		{"image opening its list item", "deploy/k8s/migrate.yaml", "        - name: migrate\n          image: ghcr.io/martialanouman/sms-platform/migrate\n", "        - image: ghcr.io/martialanouman/sms-platform/migrate\n          name: migrate\n", "not in the form deploy.sh tags"},
		{"manifest missing from deploy.sh", "deploy/test-env/deploy.sh", "for job in migrate catalog-apply; do", "for job in migrate; do", "cmd/catalog-apply: 0 deploy.sh job"},
		{"manifest twice in deploy.sh", "deploy/test-env/deploy.sh", "for job in migrate catalog-apply; do", "for job in migrate catalog-apply migrate; do", "cmd/migrate: 2 deploy.sh job"},
		{"HPA beyond its partitions", "deploy/k8s/migrate.yaml", "apiVersion: batch/v1\n", "apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nspec: {scaleTargetRef: {name: router-svc}, maxReplicas: 1000}\n---\napiVersion: batch/v1\n", "maxReplicas 1000 above the"},
		{"envFrom", "deploy/k8s/migrate.yaml", "          env:\n", "          envFrom:\n            - configMapRef: {name: x}\n          env:\n", "envFrom"},
		{"service missing from deploy.sh", "deploy/test-env/deploy.sh", "for service in content-key-svc; do", "for service in other; do", "cmd/content-key-svc: 0 deploy.sh"},
		{"Deployment among the Jobs", "deploy/test-env/deploy.sh", "for job in migrate catalog-apply; do", "for job in migrate catalog-apply content-key-svc; do", "content-key-svc: a Deployment in the loop of the Jobs"},
		{"Job among the services", "deploy/test-env/deploy.sh", "for service in content-key-svc; do", "for service in content-key-svc migrate; do", "migrate: a Job in the loop of the services"},
		{"services before the Jobs", "deploy/test-env/deploy.sh", "for job in migrate catalog-apply; do", "for service in content-key-svc; do\ndone\nfor job in migrate catalog-apply; do", "the services before the Jobs"},
		{"services without Jobs", "deploy/test-env/deploy.sh", "for job in migrate catalog-apply; do", "for jobs in migrate catalog-apply; do", "the services before the Jobs"},
		{"rules not created", "deploy/test-env/deploy.sh", "kubectl -n monitoring create configmap alert-rules", "true", "alert-rules"},
		{"rules created after the brick", "deploy/test-env/deploy.sh", "rules=()\n", "kubectl apply -f deploy/bricks/monitoring.yaml\nrules=()\n", "alert-rules"},
		{"prometheus not awaited", "deploy/test-env/deploy.sh", "kubectl -n monitoring rollout status deploy/prometheus-server", "kubectl -n monitoring get deploy/prometheus-server", "prometheus-server"},
		{"services not awaited", "deploy/test-env/deploy.sh", "-n sms rollout status", "-n sms get", "rollout status"},
		{"Service open on the node", "deploy/k8s/content-key-svc.yaml", "spec:\n  selector:", "spec:\n  type: NodePort\n  selector:", "Service type"},
		{"Service to other pods", "deploy/k8s/content-key-svc.yaml", "  selector:\n    app.kubernetes.io/name: content-key-svc", "  selector:\n    app.kubernetes.io/name: admin-api-svc", "Service selector"},
		{"Service to another port", "deploy/k8s/content-key-svc.yaml", "      port: 7002\n", "      port: 7002\n      targetPort: 7003\n", "targetPort"},
		{"Service to a named port", "deploy/k8s/content-key-svc.yaml", "      port: 7002\n", "      port: 7002\n      targetPort: grpc\n", "targetPort"},
		{"wait commented out", "deploy/test-env/deploy.sh", "\tkubectl -n sms rollout status", "\t# kubectl -n sms rollout status", "rollout status"},
		{"wait forgiven", "deploy/test-env/deploy.sh", "\"deploy/$service\" --timeout=5m\n", "\"deploy/$service\" --timeout=5m || true\n", "rollout status"},
		{"wait after the loop", "deploy/test-env/deploy.sh", "\tkubectl -n sms rollout status \"deploy/$service\" --timeout=5m\ndone\n", "done\n\tkubectl -n sms rollout status \"deploy/$service\" --timeout=5m\n", "rollout status"},
		{"loop forgiven", "deploy/test-env/deploy.sh", "--timeout=5m\ndone\necho", "--timeout=5m\ndone || true\necho", "rollout status"},
		{"Service account token mounted", "deploy/k8s/content-key-svc.yaml", "      automountServiceAccountToken: false\n", "", "automountServiceAccountToken"},
		{"Job with the API token", "deploy/k8s/migrate.yaml", "automountServiceAccountToken: false", "automountServiceAccountToken: true", "automountServiceAccountToken"},
		{"root Deployment", "deploy/k8s/content-key-svc.yaml", "runAsNonRoot: true", "runAsNonRoot: false", "runAsNonRoot"},
		{"Deployment without readiness", "deploy/k8s/content-key-svc.yaml", "path: /readyz", "path: /healthz", "readinessProbe"},
		{"grace shorter than the drain", "deploy/k8s/content-key-svc.yaml", "terminationGracePeriodSeconds: 125", "terminationGracePeriodSeconds: 124", "terminationGracePeriodSeconds"},
		{"drain lengthened past the grace", "deploy/k8s/content-key-svc.yaml", "          env:\n", "          env:\n            - name: DRAIN_BUDGET\n              value: 91s\n", "terminationGracePeriodSeconds"},
		{"Service of another name", "deploy/k8s/content-key-svc.yaml", "kind: Service\nmetadata:\n  name: content-key-svc", "kind: Service\nmetadata:\n  name: other", "Service \"other\""},
		{"Service on another port", "deploy/k8s/content-key-svc.yaml", "      port: 7002", "      port: 7003", "Service port"},
		{"ServiceAccount of another name", "deploy/k8s/content-key-svc.yaml", "kind: ServiceAccount\nmetadata:\n  name: content-key-svc", "kind: ServiceAccount\nmetadata:\n  name: other", "ServiceAccount \"other\""},
		{"allowlist beyond the method map", "deploy/k8s/content-key-svc.yaml", "value: admin-api-svc", "value: admin-api-svc, router-svc", "TLS_ALLOWED_CLIENTS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(f string, data []byte) {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, f), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.MkdirAll(filepath.Join(root, "deploy", "k8s"), 0o750); err != nil {
				t.Fatal(err)
			}
			files := []string{".goreleaser.yaml", "deploy/k8s/migrate.yaml", "cmd/migrate/main.go",
				"deploy/k8s/catalog-apply.yaml", "cmd/catalog-apply/main.go", "deploy/k8s/content-key-svc.yaml", "cmd/content-key-svc/main.go", "contracts/grants/admin-api-svc.yaml", "deploy/k8s/networkpolicies.yaml", "deploy/test-env/deploy.sh"}
			for _, f := range files {
				data, err := os.ReadFile(filepath.Join("..", f))
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case f != tc.file:
				case tc.old != "":
					if !bytes.Contains(data, []byte(tc.old)) {
						t.Fatalf("%s does not hold %q", f, tc.old)
					}
					data = bytes.Replace(data, []byte(tc.old), []byte(tc.new), 1)
				case tc.new == "":
					continue
				default:
					data = []byte(tc.new)
				}
				write(f, data)
			}
			if !slices.Contains(files, tc.file) {
				write(tc.file, []byte(tc.new))
			}
			if err := check(root); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("check = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

type listener struct {
	Port           int  `yaml:"port"`
	TLS            bool `yaml:"tls"`
	Authentication struct {
		Type           string            `yaml:"type"`
		ListenerConfig map[string]string `yaml:"listenerConfig"`
	} `yaml:"authentication"`
}

type brick struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		PostgreSQL struct {
			Parameters map[string]string `yaml:"parameters"`
		} `yaml:"postgresql"`
		Kafka struct {
			Listeners     []listener `yaml:"listeners"`
			Authorization struct {
				Type       string   `yaml:"type"`
				SuperUsers []string `yaml:"superUsers"`
			} `yaml:"authorization"`
		} `yaml:"kafka"`
		EntityOperator any `yaml:"entityOperator"`
		ClientsCa      any `yaml:"clientsCa"`
		Managed        any `yaml:"managed"`
		Bootstrap      struct {
			InitDB struct {
				PostInitSQL []string `yaml:"postInitSQL"`
			} `yaml:"initdb"`
		} `yaml:"bootstrap"`
		PodSelector struct {
			MatchLabels map[string]string `yaml:"matchLabels"`
		} `yaml:"podSelector"`
		PolicyTypes     []string `yaml:"policyTypes"`
		Ingress         []any    `yaml:"ingress"`
		Chart           string   `yaml:"chart"`
		TargetNamespace string   `yaml:"targetNamespace"`
		ValuesContent   string   `yaml:"valuesContent"`
	} `yaml:"spec"`
}

type relabel struct {
	SourceLabels []string `yaml:"source_labels"`
	Regex        string   `yaml:"regex"`
	Action       string   `yaml:"action"`
}

type mount struct {
	ConfigMap string `yaml:"configMap"`
	MountPath string `yaml:"mountPath"`
}

func rulesMount(mounts []mount) string {
	for _, m := range mounts {
		if m.ConfigMap == "alert-rules" {
			return m.MountPath
		}
	}
	return ""
}

var bricks = []string{"postgres.yaml", "kafka.yaml", "openbao.yaml", "monitoring.yaml"}

func checkBricks(dir string) error {
	var docs []brick
	for _, f := range bricks {
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
	chart := func(name string) *brick {
		for i := range docs {
			if docs[i].Kind == "HelmChart" && docs[i].Spec.Chart == name {
				return &docs[i]
			}
		}
		return nil
	}
	var errs []error
	if pg := find("Cluster"); pg == nil || pg.Metadata.Name != "postgres" || pg.Metadata.Namespace != "postgres" {
		errs = append(errs, errors.New("no Cluster postgres in namespace postgres, which internal/netpol targets"))
	} else {
		if pg.Spec.PostgreSQL.Parameters["track_commit_timestamp"] != "on" {
			errs = append(errs, errors.New("postgres: track_commit_timestamp is not on, which config_propagation_seconds reads"))
		}
		if pg.Spec.Managed != nil {
			errs = append(errs, errors.New("postgres: managed roles have their memberships reconciled by CloudNativePG, which revokes the ADMIN OPTION a creator holds (#124)"))
		}
		if !slices.Contains(pg.Spec.Bootstrap.InitDB.PostInitSQL, "ALTER ROLE migrator CREATEROLE") {
			errs = append(errs, errors.New("postgres: migrator is not given CREATEROLE at initdb, without which it creates no service role"))
		}
	}
	if bao := chart("openbao"); bao == nil || bao.Spec.TargetNamespace != "openbao" {
		errs = append(errs, errors.New("no HelmChart openbao in namespace openbao, which the NetworkPolicy of postgres admits"))
	} else {
		var v struct {
			Server struct {
				ConfigAnnotation bool `yaml:"configAnnotation"`
				Dev              struct {
					Enabled bool `yaml:"enabled"`
				} `yaml:"dev"`
				HA struct {
					Enabled bool `yaml:"enabled"`
					Raft    struct {
						Enabled bool   `yaml:"enabled"`
						Config  string `yaml:"config"`
					} `yaml:"raft"`
				} `yaml:"ha"`
			} `yaml:"server"`
		}
		if err := yaml.Unmarshal([]byte(bao.Spec.ValuesContent), &v); err != nil {
			errs = append(errs, fmt.Errorf("openbao: valuesContent unreadable: %w", err))
		} else if h := v.Server.HA; v.Server.Dev.Enabled || !h.Enabled || !h.Raft.Enabled || !strings.Contains(h.Raft.Config, `storage "raft"`) {
			errs = append(errs, errors.New("openbao: not in HA raft mode, without which no snapshot exists"))
		} else if !strings.Contains(h.Raft.Config, `audit "file"`) {
			errs = append(errs, errors.New("openbao: no audit device, which spec 07 §7.6 requires of breakglass credentials"))
		} else if !v.Server.ConfigAnnotation {
			errs = append(errs, errors.New("openbao: no config checksum on its pods, without which deploy.sh misses a configuration left unapplied"))
		}
	}
	if prom := chart("prometheus"); prom == nil || prom.Spec.TargetNamespace != "monitoring" {
		errs = append(errs, errors.New("no HelmChart prometheus in namespace monitoring, which internal/netpol and the NetworkPolicy of openbao admit"))
	} else {
		var v struct {
			ConfigmapReload struct {
				Prometheus struct {
					ExtraVolumeDirs      []string `yaml:"extraVolumeDirs"`
					ExtraConfigmapMounts []mount  `yaml:"extraConfigmapMounts"`
				} `yaml:"prometheus"`
			} `yaml:"configmapReload"`
			Server struct {
				ExtraConfigmapMounts []mount `yaml:"extraConfigmapMounts"`
			} `yaml:"server"`
			ScrapeConfigs map[string]struct {
				RelabelConfigs []relabel `yaml:"relabel_configs"`
			} `yaml:"scrapeConfigs"`
			ServerFiles struct {
				Prometheus struct {
					RuleFiles []string `yaml:"rule_files"`
				} `yaml:"prometheus.yml"`
			} `yaml:"serverFiles"`
		}
		if err := yaml.Unmarshal([]byte(prom.Spec.ValuesContent), &v); err != nil {
			errs = append(errs, fmt.Errorf("prometheus: valuesContent unreadable: %w", err))
		} else {
			if !slices.Contains(v.ServerFiles.Prometheus.RuleFiles, rulesMount(v.Server.ExtraConfigmapMounts)+"/*.yaml") {
				errs = append(errs, errors.New("prometheus: rule_files is not the mount of the ConfigMap alert-rules, which deploy.sh creates from deploy/alerts"))
			}
			if !slices.Contains(v.ConfigmapReload.Prometheus.ExtraVolumeDirs, rulesMount(v.ConfigmapReload.Prometheus.ExtraConfigmapMounts)) {
				errs = append(errs, errors.New("prometheus: the reloader does not watch the rules, which a release changes without restarting Prometheus"))
			}
			if !slices.ContainsFunc(v.ScrapeConfigs["sms"].RelabelConfigs, func(r relabel) bool {
				return slices.Equal(r.SourceLabels, []string{"__meta_kubernetes_pod_container_port_name"}) && r.Regex == "ops" && r.Action == "keep"
			}) {
				errs = append(errs, errors.New("prometheus: the job sms does not keep the port named ops, the one internal/netpol opens to Prometheus"))
			}
		}
	}
	if !slices.ContainsFunc(docs, func(b brick) bool {
		return b.Kind == "NetworkPolicy" && b.Metadata.Namespace == "monitoring" && b.Spec.PodSelector.MatchLabels == nil &&
			slices.Contains(b.Spec.PolicyTypes, "Ingress") && b.Spec.Ingress == nil
	}) {
		errs = append(errs, errors.New("monitoring: no default deny of ingress, which 06 §4 requires of every namespace"))
	}
	k := find("Kafka")
	if k == nil || k.Metadata.Name != "kafka" || k.Metadata.Namespace != "kafka" {
		return errors.Join(append(errs, errors.New("no Kafka kafka in namespace kafka, which KAFKA_BROKERS of catalog-apply names"))...)
	}
	if !slices.ContainsFunc(k.Spec.Kafka.Listeners, func(l listener) bool {
		c := l.Authentication.ListenerConfig
		return l.Port == 9093 && l.TLS && l.Authentication.Type == "custom" && c["ssl.client.auth"] == "required" &&
			strings.HasPrefix(c["ssl.truststore.location"], "/mnt/internal-ca/") && c["ssl.truststore.type"] == "PEM"
	}) {
		errs = append(errs, errors.New("kafka: no listener 9093 with TLS and a client certificate required against the internal CA, which catalog-apply dials"))
	}
	if k.Spec.Kafka.Authorization.Type != "simple" || !slices.Contains(k.Spec.Kafka.Authorization.SuperUsers, "CN=catalog-apply") {
		errs = append(errs, errors.New("kafka: authorization is not simple with superUsers CN=catalog-apply"))
	}
	if k.Spec.ClientsCa != nil {
		errs = append(errs, errors.New("kafka: a clientsCa would need the root key copied out of cert-manager"))
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
		{"commit timestamps off", "postgres.yaml", `track_commit_timestamp: "on"`, `track_commit_timestamp: "off"`, "track_commit_timestamp"},
		{"postgres elsewhere", "postgres.yaml", "  name: postgres\n  namespace: postgres\nspec:\n  instances", "  name: postgres\n  namespace: db\nspec:\n  instances", "no Cluster postgres"},
		{"kafka renamed", "kafka.yaml", "kind: Kafka\nmetadata:\n  name: kafka", "kind: Kafka\nmetadata:\n  name: events", "no Kafka kafka"},
		{"listener on another port", "kafka.yaml", "port: 9093", "port: 9094", "no listener 9093"},
		{"listener without mTLS", "kafka.yaml", "authentication:\n          type: custom", "authentication:\n          type: scram-sha-512", "no listener 9093"},
		{"truststore of the JVM", "kafka.yaml", "ssl.truststore.location: /mnt/internal-ca/ca.crt", "ssl.truststore.location: /etc/pki/java/cacerts", "no listener 9093"},
		{"truststore not PEM", "kafka.yaml", "ssl.truststore.type: PEM", "ssl.truststore.type: JKS", "no listener 9093"},
		{"client certificate optional", "kafka.yaml", "ssl.client.auth: required", "ssl.client.auth: requested", "no listener 9093"},
		{"catalog-apply not super", "kafka.yaml", "- CN=catalog-apply", "- CN=admin", "superUsers CN=catalog-apply"},
		{"kafka elsewhere", "kafka.yaml", "  name: kafka\n  namespace: kafka\nspec:\n  kafka:", "  name: kafka\n  namespace: events\nspec:\n  kafka:", "no Kafka kafka"},
		{"listener without TLS", "kafka.yaml", "        tls: true\n        authentication:", "        tls: false\n        authentication:", "no listener 9093"},
		{"authorization not simple", "kafka.yaml", "      type: simple", "      type: opa", "authorization is not simple"},
		{"entity operator", "kafka.yaml", "      transaction.state.log.min.isr: 2\n", "      transaction.state.log.min.isr: 2\n  entityOperator:\n    topicOperator: {}\n", "entityOperator"},
		{"clients CA provided", "kafka.yaml", "      transaction.state.log.min.isr: 2\n", "      transaction.state.log.min.isr: 2\n  clientsCa:\n    generateCertificateAuthority: false\n", "clientsCa"},
		{"roles managed by CloudNativePG", "postgres.yaml", "  certificates:\n", "  managed:\n    roles:\n      - name: openbao\n        ensure: present\n  certificates:\n", "managed roles"},
		{"migrator without CREATEROLE", "postgres.yaml", "        - ALTER ROLE migrator CREATEROLE\n", "", "CREATEROLE"},
		{"openbao chart renamed", "openbao.yaml", "  chart: openbao\n", "  chart: vault\n", "no HelmChart openbao"},
		{"openbao elsewhere", "openbao.yaml", "  targetNamespace: openbao\n", "  targetNamespace: kms\n", "no HelmChart openbao"},
		{"openbao without raft", "openbao.yaml", "        raft:\n          enabled: true", "        raft:\n          enabled: false", "not in HA raft mode"},
		{"openbao standalone", "openbao.yaml", "      ha:\n        enabled: true", "      ha:\n        enabled: false", "not in HA raft mode"},
		{"openbao in dev mode", "openbao.yaml", "    server:\n", "    server:\n      dev:\n        enabled: true\n", "not in HA raft mode"},
		{"openbao on file storage", "openbao.yaml", `storage "raft" {`, `storage "file" {`, "not in HA raft mode"},
		{"openbao config not checksummed", "openbao.yaml", "      configAnnotation: true\n", "", "config checksum"},
		{"openbao without audit", "openbao.yaml", `audit "file" "stdout"`, `audit "socket" "stdout"`, "no audit device"},
		{"openbao values unreadable", "openbao.yaml", "    global:\n", "    global: [\n", "valuesContent unreadable"},
		{"prometheus renamed", "monitoring.yaml", "  chart: prometheus\n", "  chart: kube-prometheus-stack\n", "no HelmChart prometheus"},
		{"prometheus elsewhere", "monitoring.yaml", "  targetNamespace: monitoring\n", "  targetNamespace: observability\n", "no HelmChart prometheus"},
		{"rules on another path", "monitoring.yaml", "          - /etc/alerts/*.yaml\n", "          - /etc/config/alerting_rules.yml\n", "rule_files"},
		{"rules from another ConfigMap", "monitoring.yaml", "      extraConfigmapMounts:\n        - name: alert-rules\n          configMap: alert-rules\n", "      extraConfigmapMounts:\n        - name: alert-rules\n          configMap: rules\n", "rule_files"},
		{"rules mounted elsewhere", "monitoring.yaml", "          mountPath: /etc/alerts\n        - name: internal-ca", "          mountPath: /etc/rules\n        - name: internal-ca", "rule_files"},
		{"reloader without the rules", "monitoring.yaml", "        extraConfigmapMounts:\n          - name: alert-rules\n            configMap: alert-rules\n            mountPath: /etc/alerts\n", "", "reload"},
		{"ops port not scraped", "monitoring.yaml", "            regex: ops\n", "            regex: metrics\n", "port named ops"},
		{"ops on another label", "monitoring.yaml", "[__meta_kubernetes_pod_container_port_name]\n            regex: ops", "[__meta_kubernetes_pod_label_app]\n            regex: ops", "port named ops"},
		{"ops port dropped", "monitoring.yaml", "            regex: ops\n            action: keep", "            regex: ops\n            action: drop", "port named ops"},
		{"monitoring opened by a rule", "monitoring.yaml", "  policyTypes:\n    - Ingress\n---", "  policyTypes:\n    - Ingress\n  ingress:\n    - {}\n---", "deny"},
		{"monitoring partly denied", "monitoring.yaml", "  namespace: monitoring\nspec:\n  podSelector: {}", "  namespace: monitoring\nspec:\n  podSelector:\n    matchLabels:\n      app.kubernetes.io/name: alertmanager", "deny"},
		{"deny in another namespace", "monitoring.yaml", "  name: default-deny\n  namespace: monitoring", "  name: default-deny\n  namespace: sms", "deny"},
		{"monitoring open", "monitoring.yaml", "  name: default-deny\n  namespace: monitoring\nspec:\n  podSelector: {}\n  policyTypes:\n    - Ingress", "  name: default-deny\n  namespace: monitoring\nspec:\n  podSelector: {}\n  policyTypes:\n    - Egress", "deny"},
		{"rules not reloaded", "monitoring.yaml", "        extraVolumeDirs:\n          - /etc/alerts\n", "        extraVolumeDirs:\n          - /etc/rules\n", "reload"},
		{"prometheus values unreadable", "monitoring.yaml", "    server:\n", "    server: [\n", "valuesContent unreadable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range bricks {
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

func TestOnlyGroupAssignmentsBoundReplicas(t *testing.T) {
	c, err := catalog.Parse([]byte(`
topics:
  small: {key: manual, order: per_partition, duplicate_unit: s, payload: sms.v1.A, body: none, partitions: 2, retention: 96h, producers: [p]}
  tiny: {key: k, order: none, duplicate_unit: s, payload: sms.v1.A, body: none, partitions: 1, retention: 96h, producers: [p]}
  large: {key: k, order: none, duplicate_unit: s, payload: sms.v1.A, body: none, partitions: 64, retention: 96h, producers: [p]}
  medium: {key: k, order: none, duplicate_unit: s, payload: sms.v1.A, body: none, partitions: 32, retention: 96h, producers: [p]}
groups:
  - {topic: tiny, service: other, tenant: none, dependencies: {kafka: {mode: blocking}}, concurrency: per_partition, dead_letter: other.tiny.dead-letter, start: earliest}
  - {topic: medium, service: s, tenant: none, dependencies: {kafka: {mode: blocking}}, concurrency: per_partition, dead_letter: s.medium.dead-letter, start: earliest}
  - {topic: small, service: s, assignment: manual, tenant: none, dependencies: {kafka: {mode: blocking}}, concurrency: per_partition, dead_letter: s.small.dead-letter, start: earliest}
  - {topic: tiny, service: s, assignment: broadcast, tenant: none, dependencies: {kafka: {mode: blocking}}, concurrency: per_partition, dead_letter: s.tiny.dead-letter, start: earliest}
  - {topic: large, service: s, tenant: none, dependencies: {kafka: {mode: blocking}}, concurrency: per_partition, dead_letter: s.large.dead-letter, start: earliest}
`))
	if err != nil {
		t.Fatal(err)
	}
	if bound, by := partitionsBound(c, "s"); bound != 32 || by != "medium" {
		t.Errorf("partitionsBound = %d by %s, want 32 by medium", bound, by)
	}
}
