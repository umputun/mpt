# Get the latest commit branch, hash, and date
TAG=$(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
BRANCH=$(if $(TAG),$(TAG),$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))
HASH=$(shell git rev-parse --short=7 HEAD 2>/dev/null)
TIMESTAMP=$(shell git log -1 --format=%ct HEAD 2>/dev/null | xargs -I{} date -u -r {} +%Y%m%dT%H%M%S)
GIT_REV=$(shell printf "%s-%s-%s" "$(BRANCH)" "$(HASH)" "$(TIMESTAMP)")
REV=$(if $(filter --,$(GIT_REV)),latest,$(GIT_REV)) # fallback to latest if not in git repo

PACKAGE := github.com/umputun/mpt

all: test build

build:
	cd cmd/mpt && go build -ldflags "-X main.revision=$(REV) -s -w" -o ../../.bin/mpt.$(BRANCH)
	cp .bin/mpt.$(BRANCH) .bin/mpt

test:
	go clean -testcache
	go test -race -coverprofile=coverage.out ./...
	grep -v "_mock.go" coverage.out | grep -v mocks > coverage_no_mocks.out
	go tool cover -func=coverage_no_mocks.out
	rm coverage.out coverage_no_mocks.out

lint:
	golangci-lint run

version:
	@echo "branch: $(BRANCH), hash: $(HASH), timestamp: $(TIMESTAMP)"
	@echo "revision: $(REV)"

prep_site:
	cp -fv README.md site/docs/index.md
	sed -i '' 's|https:\/\/github.com\/umputun\/mpt\/raw\/master\/site\/docs\/logo.png|logo.png|' site/docs/index.md
	sed -i '' 's|site\/docs\/logo\.png|logo.png|g' site/docs/index.md
	sed -i '' 's|site\/docs\/logo-inverted\.png|logo-inverted.png|g' site/docs/index.md
	sed -i '' 's|^.*/workflows/ci.yml.*$$||' site/docs/index.md

.PHONY: build test lint version prep_site