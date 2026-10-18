SHELL := /bin/bash -o pipefail

.PHONY: help lint test ciguard check-generated debtguard catalog proto check

help: ## Liste les cibles
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "%-16s %s\n", $$1, $$2}'

lint: ## golangci-lint, gofmt compris
	go tool golangci-lint run ./...

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

check: lint ciguard check-generated debtguard catalog proto ## Ce que vérifie la CI
