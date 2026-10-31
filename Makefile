VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%d)
DISTRIBUTION ?=
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE) -X github.com/BSH2409/hunkyard/internal/server.buildDistribution=$(DISTRIBUTION)

build: generate
	go build -ldflags "$(LDFLAGS)" -o hunkyard ./cmd/hunkyard

generate:
	go generate ./...

verify-generate:
	go generate ./...
	git diff --exit-code cmd/hunkyard/integration_hashes_gen.go || (echo "ERROR: integration_hashes_gen.go is stale. Run 'go generate ./...' and commit." && exit 1)

build-all:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/hunkyard-darwin-arm64 ./cmd/hunkyard
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/hunkyard-darwin-amd64 ./cmd/hunkyard
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/hunkyard-linux-amd64 ./cmd/hunkyard
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/hunkyard-linux-arm64 ./cmd/hunkyard
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/hunkyard-windows-amd64.exe ./cmd/hunkyard
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/hunkyard-windows-arm64.exe ./cmd/hunkyard

update-deps:
	npm install --ignore-scripts
	npm run update-deps

test:
	go test ./...

test-frontend:
	npm run test:frontend

verify-assets:
	npm run verify-assets:installed
	npm run update-deps
	git diff --exit-code -- web/dompurify.min.js web/markdown-it.min.js web/mermaid.min.js web/katex web/gemoji.min.js web/diff-match-patch.min.js web/pierre
	test -z "$$(git status --porcelain -- web/pierre web/katex)"

# Run Go benchmarks locally. Compare against a base with:
#   git worktree add /tmp/hunkyard-base origin/main
#   go test -run='^$' -bench=. -benchmem -count=6 ./internal/diff/ ./internal/session/ > old.txt  (in /tmp/hunkyard-base)
#   go test -run='^$' -bench=. -benchmem -count=6 ./internal/diff/ ./internal/session/ > new.txt  (here)
#   benchstat old.txt new.txt   (go install golang.org/x/perf/cmd/benchstat@latest)
bench:
	go test -run='^$' -bench=. -benchmem -count=6 ./internal/diff/ ./internal/session/

bench-compare:
	benchstat bench-old.txt bench-new.txt | tee benchstat.txt
	python3 scripts/bench-compare.py benchstat.txt

setup-hooks:
	git config core.hooksPath .githooks

test-diff:
	./test/shell/test-diff.sh

test-live-cdp:
	go test -tags integration -run TestLiveCDPIntegration -v -count=1 ./internal/live/...

e2e-roundtrip: build
	./scripts/e2e-roundtrip.sh

test-daemon:
	./test/shell/test-daemon-reuse.sh

test-plan-daemon:
	./test/shell/test-plan-daemon.sh

clean:
	rm -f hunkyard
	rm -rf dist

e2e:
	cd test/e2e && bash run.sh

e2e-failed:
	cd test/e2e && npx playwright test --last-failed

e2e-report:
	cd test/e2e && npx playwright show-report

e2e-live-utils:
	node --test web/__tests__/*.test.js

test-preview: build
	@echo "Starting preview mode with sample page..."
	./hunkyard preview test/preview-sample/index.html

.PHONY: build build-all generate verify-generate update-deps test test-frontend verify-assets setup-hooks clean test-diff test-live-cdp e2e-roundtrip test-daemon test-plan-daemon e2e e2e-failed e2e-report e2e-live-utils test-preview
