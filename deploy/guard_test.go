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

	"github.com/martialanouman/sms-platform/internal/platform/config"
)

const registry = "ghcr.io/martialanouman/sms-platform/"

type goreleaser struct {
	Builds []struct {
		ID   string `yaml:"id"`
		Main string `yaml:"main"`
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
	} `yaml:"securityContext"`
	Env []struct {
		Name      string `yaml:"name"`
		ValueFrom struct {
			SecretKeyRef *struct{} `yaml:"secretKeyRef"`
		} `yaml:"valueFrom"`
	} `yaml:"env"`
	EnvFrom []any `yaml:"envFrom"`
}

type workload struct {
	Kind string `yaml:"kind"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers     []container `yaml:"containers"`
				InitContainers []container `yaml:"initContainers"`
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
	}
	for _, d := range g.Dockers {
		images = append(images, d.ID)
		if !slices.Equal(d.IDs, []string{d.ID}) || !slices.Equal(d.Images, []string{registry + d.ID}) {
			errs = append(errs, fmt.Errorf("image %s: ids %v and images %v, want [%s] and [%s%s]", d.ID, d.IDs, d.Images, d.ID, registry, d.ID))
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "deploy", "k8s", "*.yaml"))
	if err != nil {
		return err
	}
	var manifests []string
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".yaml")
		manifests = append(manifests, name)
		errs = append(errs, checkManifest(f, name)...)
	}
	errs = append(errs, same("build", cmds, builds)...)
	errs = append(errs, same("image", cmds, images)...)
	errs = append(errs, same("manifest", cmds, manifests)...)
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

func checkManifest(path, name string) []error {
	data, err := os.ReadFile(path)
	if err != nil {
		return []error{err}
	}
	var errs []error
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var w workload
		if err := dec.Decode(&w); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return append(errs, fmt.Errorf("%s: %w", path, err))
		}
		pod := w.Spec.Template.Spec
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
			}
		}
	}
	return errs
}

func TestTheTreeHoldsTheDeliveryRules(t *testing.T) {
	if err := check(".."); err != nil {
		t.Fatal(err)
	}
}

func TestEachDeliveryRuleFails(t *testing.T) {
	for _, tc := range []struct {
		name, file, old, new, want string
	}{
		{"cmd without build", ".goreleaser.yaml", "id: migrate\n    main:", "id: other\n    main:", "cmd/migrate: 0 build"},
		{"build without cmd", ".goreleaser.yaml", "id: migrate\n    main:", "id: other\n    main:", "build other: no cmd/other"},
		{"duplicate build", ".goreleaser.yaml", "builds:\n", "builds:\n  - id: migrate\n    main: ./cmd/migrate\n", "cmd/migrate: 2 build"},
		{"build of another main", ".goreleaser.yaml", "main: ./cmd/migrate", "main: ./cmd/other", "main is"},
		{"cmd without image", ".goreleaser.yaml", "dockers_v2:\n  - id: migrate", "dockers_v2:\n  - id: other", "cmd/migrate: 0 image"},
		{"image of another build", ".goreleaser.yaml", "ids: [migrate]", "ids: [other]", "image migrate: ids"},
		{"image in another repository", ".goreleaser.yaml", "images: [ghcr.io/martialanouman/sms-platform/migrate]", "images: [ghcr.io/other/migrate]", "image migrate: ids"},
		{"wrong image in manifest", "deploy/k8s/migrate.yaml", "image: ghcr.io/martialanouman/sms-platform/migrate", "image: ghcr.io/martialanouman/sms-platform/migrate:latest", "image \""},
		{"root container", "deploy/k8s/migrate.yaml", "runAsNonRoot: true", "runAsNonRoot: false", "runAsNonRoot"},
		{"writable root filesystem", "deploy/k8s/migrate.yaml", "readOnlyRootFilesystem: true", "readOnlyRootFilesystem: false", "readOnlyRootFilesystem"},
		{"variable config does not read", "deploy/k8s/migrate.yaml", "- name: POSTGRES_URL", "- name: DATABASE_URL", "DATABASE_URL is not read by config"},
		{"literal URL", "deploy/k8s/migrate.yaml", "valueFrom:\n                secretKeyRef:\n                  name: migrate\n                  key: postgres-url", "value: postgres://x", "secretKeyRef"},
		{"envFrom", "deploy/k8s/migrate.yaml", "          env:\n", "          envFrom:\n            - configMapRef: {name: x}\n          env:\n", "envFrom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, f := range []string{".goreleaser.yaml", "deploy/k8s/migrate.yaml", "cmd/migrate/main.go"} {
				data, err := os.ReadFile(filepath.Join("..", f))
				if err != nil {
					t.Fatal(err)
				}
				if f == tc.file {
					if !bytes.Contains(data, []byte(tc.old)) {
						t.Fatalf("%s does not hold %q", f, tc.old)
					}
					data = bytes.Replace(data, []byte(tc.old), []byte(tc.new), 1)
				}
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, f), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := check(root); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("check = %v, want an error with %q", err, tc.want)
			}
		})
	}
}

func TestACmdWithoutManifestFails(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{".goreleaser.yaml", "cmd/migrate/main.go"} {
		data, err := os.ReadFile(filepath.Join("..", f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, f)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, f), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := check(root); err == nil || !strings.Contains(err.Error(), "cmd/migrate: 0 manifest") {
		t.Fatalf("check = %v, want a missing manifest", err)
	}
}
