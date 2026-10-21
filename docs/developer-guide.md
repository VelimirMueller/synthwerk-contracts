# Developer guide

> **In 30 seconds**
> - Change the contract first, then the service code (plan §6, condition 1).
> - `just` runs lint, codegen and every test. CI runs the same commands.
> - Never edit a published name or a generated file.

## Needs

- Go 1.27 (`go.mod` pins the toolchain), Node 24, pnpm 12, [just](https://just.systems).
- golangci-lint v2, actionlint and uv (for zizmor) for `just lint`.
- `pnpm install` once. Go tools (oapi-codegen, oasdiff) come from `go.mod` (`go tool …`).

## Layout

| Path | Holds |
|---|---|
| `openapi/` | OpenAPI 3.1 specs. `_shared/` holds the parts every service uses. |
| `events/` | CloudEvents envelope, catalog, data schemas per domain, one example per type. |
| `rules/` | Spectral ruleset, oasdiff severity levels. |
| `codegen/` | oapi-codegen config per spec. |
| `go/` | Go module `github.com/VelimirMueller/synthwerk-contracts/go`. Generated. |
| `packages/ts/` | npm package `@synthwerk/contracts`. `src/gen/` is generated. |
| `tools/eventcompat/` | The event breaking-change check (Go). |
| `tests/unit/`, `tests/contract/` | Go tests (D-15). `tests/fixtures/` feeds the selftests. |
| `scripts/` | `gen.sh`, `check-breaking.sh`, the selftests. |

- The Go module sits in `go/`, not `packages/go/`: a Go submodule path must match its folder, and tags are `go/vX.Y.Z`.
- The repo root is a second Go module for the tools and tests. It is never published.

## Commands

| Command | Does |
|---|---|
| `just` | lint, gen, test |
| `just lint` | Spectral, Biome, golangci-lint, actionlint, zizmor |
| `just gen` | Go and TS clients from the specs |
| `just test-unit` | `eventcompat` rules |
| `just test-contract` | catalog, schemas, examples, generated Go client |
| `just compile` | `go vet` + `go build` in `go/`, `tsc --noEmit` in `packages/ts/` |
| `just selftest` | the gates fail on a renamed required field and on each Spectral rule |
| `just breaking origin/main` | breaking-change check against a ref |

## Change a contract

1. Edit the spec or the schema. Follow `docs/naming.md`.
2. Run `just gen` and commit the generated files with the spec.
3. Update the example and the catalog entry for an event change.
4. Update the feature doc in `docs/features/`. CI checks it.
5. Run `just`. Push. CI runs the breaking-change check against the PR base.

## Add a service API

1. Add `openapi/<service>/openapi.yaml`. Reference `_shared/health.yaml` for the probes.
2. Add `codegen/oapi-codegen.<service>.yaml` with `output: go/<service>/<service>.gen.go`.
3. Add a `gen` line for the TS types in `packages/ts/package.json` and export a client in `src/index.ts`.

## CI

| Check | Job |
|---|---|
| `ci / required` | blueprint `go-ci.yml@v1`: gofmt, golangci-lint, `go test`, `go build` |
| `lint` | Spectral, Spectral selftest, Biome |
| `codegen` | generate, fail on drift, compile Go and TS |
| `breaking` | `check-breaking.sh` against the PR base, breaking selftest |
| `gate` | all of the above |
| `feature-doc / check` | blueprint feature-doc check (D-16) |

## Not yet here

- Publish: npm (`@synthwerk/contracts`), Go tags `go/vX.Y.Z`, release-please.
- Python models, Go and TS types for event data, the shared kit (issue #25), MCP manifests.
