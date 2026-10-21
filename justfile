# Tasks for synthwerk-contracts. CI runs the same commands (.github/workflows/ci.yml).
set shell := ["bash", "-euo", "pipefail", "-c"]

# Lint, generate, test.
default: lint gen test

zizmor_version := "1.30.1"

# Spectral on the specs, Biome on JSON and TS, golangci-lint on Go, actionlint + zizmor on workflows.
lint:
    actionlint .github/workflows/*.yml
    uvx zizmor@{{ zizmor_version }} --offline .github/workflows
    pnpm run lint:api
    pnpm exec biome ci .
    golangci-lint run ./...

# Generate the Go and TS clients from the specs.
gen:
    scripts/gen.sh

# All tests: Go unit + contract tests, generated clients compile, both selftests.
test: test-unit test-contract compile selftest

# eventcompat rules (tests/unit).
test-unit:
    go test ./tests/unit/...

# Catalog, schemas, examples and the generated Go client (tests/contract).
test-contract:
    go test ./tests/contract/...

# The generated clients compile.
compile:
    cd go && go vet ./... && go build ./...
    pnpm run typecheck

# The gates catch what they promise: a renamed required field, every Spectral rule.
selftest:
    scripts/selftest-breaking.sh
    scripts/selftest-spectral.sh

# Breaking-change check against a git ref or a dir.
breaking base="origin/main":
    scripts/check-breaking.sh {{ base }}
