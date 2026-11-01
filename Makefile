# make check is what CI runs. The dev tools are pinned in tools/go.mod.
TOOL       := go tool -modfile=tools/go.mod
GORELEASER := $(shell command -v goreleaser 2>/dev/null || echo go run github.com/goreleaser/goreleaser/v2@v2.19.0)

.PHONY: check fmt-check lint test cover vuln release-check fmt golden docs build

check: fmt-check lint test vuln release-check

# Go stamps a build with the nearest .git folder, and a worktree's .git is a
# file: inside the main checkout, it would stamp main's commit. Say which.
build:
	go build -ldflags "-X github.com/farzeni/skl/internal/cli.commit=$$(git rev-parse --short HEAD 2>/dev/null)" -o bin/skl ./cmd/skl

test:
	go test -race ./...

# Coverage of all of skl: -coverpkg counts what the spec scripts reach through
# the skl they run, not only each package's own statements.
cover:
	@out=$$(mktemp); go test -coverpkg=./... -coverprofile=$$out ./... && go tool cover -func=$$out | tail -n 1; status=$$?; rm -f $$out; exit $$status

fmt-check:
	@out=$$($(TOOL) gofumpt -l .); if [ -n "$$out" ]; then echo "not formatted (make fmt):"; echo "$$out"; exit 1; fi

lint:
	$(TOOL) golangci-lint run

vuln:
	$(TOOL) govulncheck ./...

release-check:
	$(GORELEASER) check

fmt:
	$(TOOL) golangci-lint fmt

# Rewrites golden files (testdata/*.golden) and the expected output in spec
# scripts: every test that compares against a file honors UPDATE=1.
golden:
	UPDATE=1 go test ./...

docs:
	cd docs && pnpm install --frozen-lockfile && pnpm build
