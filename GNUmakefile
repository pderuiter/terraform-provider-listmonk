default: fmt lint build test

build:
	go build -v ./...

install: build
	go install -v ./...

lint:
	golangci-lint run

# Regenerate docs/ from the schemas, examples/ and templates/. CI fails when
# the committed docs differ from the generated ones.
generate:
	go generate ./...

fmt:
	gofmt -s -w -e .
	terraform fmt -recursive ./examples/

test:
	go test -v -cover -race -timeout=120s ./...

# Acceptance tests against a throwaway listmonk (PostgreSQL in Docker, or
# PG_MODE=native for a local PostgreSQL). LISTMONK_VERSION picks the release.
testacc:
	scripts/testacc-local.sh test

.PHONY: default build install lint generate fmt test testacc
