.PHONY: help build test vet lint validate-scripts install-hooks

help:
	@printf "Beads Workbench convenience targets (thin wrappers):\n"
	@printf "  help              Show this help\n"
	@printf "  build             go build ./cmd/bwb\n"
	@printf "  test              go test ./...\n"
	@printf "  vet               go vet ./...\n"
	@printf "  lint              Pinned golangci-lint run (optional wrapper)\n"
	@printf "  validate-scripts  Validate repo helper scripts (optional wrapper)\n"
	@printf "  install-hooks     Configure repo-managed git hooks path (optional wrapper)\n"
	@printf "\nSee docs/CODING.md for the authoritative quality-gate sequence.\n"

build:
	go build ./cmd/bwb

test:
	go test ./...

vet:
	go vet ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@"$$(cat .golangci-version)" run

validate-scripts:
	bash -n internal/testing/e2e/embeddedfixture/setup.sh
	python3 -m py_compile scripts/*.py

install-hooks:
	git config core.hooksPath scripts/git-hooks
