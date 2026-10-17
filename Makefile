SHELL := /bin/bash -o pipefail

.PHONY: help lint test ciguard check-generated check

help: ## Liste les cibles
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "%-16s %s\n", $$1, $$2}'

lint: ## golangci-lint, gofmt compris
	go tool golangci-lint run ./...

test: ## Tests avec -race ; un test peut sauter
	go test -race ./...

ciguard: ## Tests avec -race ; un test qui saute échoue
	go test -race -v ./... | tee /dev/stderr | { ! grep -- '--- SKIP'; }

check-generated: ## La génération ne change rien à l'arbre
	@before="$$(git status --porcelain -uall; git diff)"; go generate ./... || exit 1; \
		[ "$$before" = "$$(git status --porcelain -uall; git diff)" ] \
		|| { echo "go generate a modifié l'arbre :"; git status --porcelain -uall; exit 1; }

check: lint ciguard check-generated ## Ce que vérifie la CI
