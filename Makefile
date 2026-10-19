SHELL := /bin/bash -o pipefail

.PHONY: help lint vuln test ciguard check-generated debtguard catalog proto openapi alerts image scan check

# Hors de la directive tool (décision 9, C1) : une version ici, la seule. GoReleaser exige Go 1.27 ; setup-go pose
# GOTOOLCHAIN=local en CI.
GORELEASER := GOTOOLCHAIN=auto go run github.com/goreleaser/goreleaser/v2@v2.18.3
GRYPE := go run github.com/anchore/grype/cmd/grype@v0.120.1
# Le module de Prometheus porte des replace, que go run refuse : son image officielle, épinglée par digest.
PROMTOOL := docker run --rm -v $(CURDIR)/deploy/alerts:/alerts:ro -w /alerts --entrypoint promtool \
	prom/prometheus:v3.15.0@sha256:efd719c99d83b060d9daefdcf00360461adf279f45ef5391f8d111892118753e

help: ## Liste les cibles
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "%-16s %s\n", $$1, $$2}'

lint: ## golangci-lint, gofmt compris ; une exclusion vit dans .golangci.yml, jamais en nolint ni go:linkname
	go tool golangci-lint run ./...
	@! git grep --untracked -nE 'nolint|go:linkname' -- '*.go'

vuln: ## govulncheck : une vulnérabilité connue atteinte par le code échoue
	go tool govulncheck ./...

test: ## Tests avec -race ; un test peut sauter
	go test -race ./...

# debt: #2
ciguard: ## Tests avec -race ; un test qui saute échoue
	go test -race -v ./... | tee /dev/stderr | { ! grep -- '--- SKIP'; }

check-generated: ## La génération ne change rien à l'arbre
	@before="$$(git status --porcelain -uall; git diff)"; go generate ./... || exit 1; \
		[ "$$before" = "$$(git status --porcelain -uall; git diff)" ] \
		|| { echo "go generate a modifié l'arbre :"; git status --porcelain -uall; exit 1; }

debtguard: ## Chaque marque de dette désigne une issue ouverte étiquetée debt
	GITHUB_REPOSITORY=$${GITHUB_REPOSITORY:-martialanouman/sms-platform} \
		GH_TOKEN=$${GH_TOKEN:-$$(gh auth token 2>/dev/null)} go run ./tools/debtguard

catalog: ## Le catalogue des topics est valide et ses partitions per_key n'ont pas bougé
	go run ./tools/catalog

proto: ## buf lint, puis buf breaking contre origin/main
	go tool buf lint
	@base=$${PROTO_BASE:-origin/main}; git rev-parse --verify --quiet "$$base^{commit}" >/dev/null \
		|| { echo "base $$base introuvable"; exit 1; }; \
	if [ -z "$$(git ls-tree --name-only "$$base" -- contracts/proto)" ]; then \
		echo "buf breaking : $$base n'a pas de contracts/proto, rien à comparer"; \
	else go tool buf breaking --against ".git#ref=$$base"; fi

openapi: ## Gardes des contrats, oasdiff validate, puis oasdiff breaking contre origin/main (bloquant pour le public)
	go run ./tools/openapi
	go tool oasdiff validate --fail-on WARN contracts/openapi-public.yaml
	go tool oasdiff validate --fail-on WARN contracts/openapi-admin.yaml
	go tool oasdiff validate --fail-on WARN contracts/openapi-bff.yaml
	@base=$${OPENAPI_BASE:-origin/main}; git rev-parse --verify --quiet "$$base^{commit}" >/dev/null \
		|| { echo "base $$base introuvable"; exit 1; }; \
	for f in contracts/openapi-public.yaml contracts/openapi-admin.yaml contracts/openapi-bff.yaml; do \
		if [ -z "$$(git ls-tree --name-only "$$base" -- $$f)" ]; then \
			echo "oasdiff breaking : $$base n'a pas de $$f, rien à comparer"; continue; fi; \
		case $$f in *public*) fail="--fail-on ERR";; *) fail="";; esac; \
		go tool oasdiff breaking $$fail "$$base:$$f" $$f || exit 1; \
	done

alerts: ## promtool : chaque fichier de règles se charge, chaque test de règles passe
	$(PROMTOOL) check rules $$(cd deploy/alerts && ls *.yaml | grep -v '_test\.yaml$$')
	$(PROMTOOL) test rules $$(cd deploy/alerts && ls *_test.yaml)

image: ## Images de chaque cmd/, en snapshot, sans push
	$(GORELEASER) release --snapshot --clean

scan: image ## grype sur chaque image du snapshot : une vulnérabilité connue échoue, aucune image aussi
	@images=$$(jq -r '.[] | select(.type == "Docker Image") | .name' dist/artifacts.json) || exit 1; \
	[ -n "$$images" ] || { echo "aucune image à scanner"; exit 1; }; \
	for i in $$images; do $(GRYPE) --fail-on low "docker:$$i" || exit 1; done

check: lint vuln ciguard check-generated debtguard catalog proto openapi alerts scan ## Ce que vérifie la CI
