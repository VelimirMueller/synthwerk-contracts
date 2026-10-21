<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/banner/hero-v2-light.svg">
  <img alt="SYNTHWERK-CONTRACTS. The promises, in writing. Break one and CI turns red. Draft, M1." src="assets/banner/hero-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/lead-v2-light.svg">
  <img alt="synthwerk-contracts is the one contract of Synthwerk: OpenAPI 3.1 for HTTP, CloudEvents 1.0.2 with JSON Schema for events. CI generates the Go and TypeScript clients and fails every pull request that breaks a consumer. It runs nothing. It only makes promises." src="assets/readme/lead-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/stats-v2-light.svg">
  <img alt="2 HEALTH ENDPOINTS. 2 EVENT TYPES. 6 CUSTOM LINT RULES. 2 GENERATED CLIENTS" src="assets/readme/stats-v2-dark.svg" width="100%">
</picture>
</p>

<br>

<p>
<img alt="01 WHAT IT DOES. ONE CONTRACT. EVERY CLIENT." src="assets/readme/divider-what-v2.svg" width="100%">
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/features-v2-light.svg">
  <img alt="ONE SOURCE: Every API and every event is defined here first. Services follow the contract. GENERATED CLIENTS: Go (oapi-codegen) and TypeScript (openapi-fetch). CI fails when they drift. BREAKING = RED: oasdiff and eventcompat fail a PR that renames or removes a field" src="assets/readme/features-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/what-v2-light.svg">
  <img alt="Health API: GET /healthz and GET /readyz for every service. Pulse reads both. Event envelope: CloudEvents 1.0.2 plus tenantid, traceparent and actor. Event catalog: Per type: subject, stream, producer, consumers, status, schema, example. Naming rule: dev.synthwerk.&lt;domain&gt;.&lt;entity&gt;.&lt;verb&gt;.v&lt;N&gt;, verb in the past tense" src="assets/readme/what-v2-dark.svg" width="100%">
</picture>
</p>

<br>

<p>
<img alt="02 QUICK START. THREE COMMANDS. NO SERVER." src="assets/readme/divider-start-v2.svg" width="100%">
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/start-v2-light.svg">
  <img alt="Terminal: $ pnpm install | $ just | $ just breaking origin/main | # lint, gen, test: all green" src="assets/readme/start-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/setup-v2-light.svg">
  <img alt="pnpm install: Spectral, Biome, openapi-typescript and TypeScript from pnpm-lock.yaml. just: Lint, generate the Go and TS clients, run every test and both selftests. just breaking origin/main: Check your branch against main for breaking changes" src="assets/readme/setup-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/startnotes-v2-light.svg">
  <img alt="Needs: Go 1.27, Node 24, pnpm 12, just, golangci-lint v2. Go tools: oapi-codegen and oasdiff come from go.mod as go tool. Nothing to install" src="assets/readme/startnotes-v2-dark.svg" width="100%">
</picture>
</p>

<details>
<summary><picture><source media="(prefers-color-scheme: light)" srcset="assets/readme/links-7-v2-light.svg"><img alt="Plain text" src="assets/readme/links-7-v2-dark.svg" width="32%"></picture></summary>

```bash
pnpm install                 # Spectral, Biome, openapi-typescript, TypeScript
just                         # lint, gen, test
just breaking origin/main    # breaking-change check against main
```

</details>

<p align="center">
<a href="docs/naming.md"><picture><source media="(prefers-color-scheme: light)" srcset="assets/readme/links-1-v2-light.svg"><img alt="Naming (docs/naming.md)" src="assets/readme/links-1-v2-dark.svg" width="32%"></picture></a>
<a href="docs/developer-guide.md"><picture><source media="(prefers-color-scheme: light)" srcset="assets/readme/links-2-v2-light.svg"><img alt="Guide (docs/developer-guide.md)" src="assets/readme/links-2-v2-dark.svg" width="32%"></picture></a>
<a href="docs/features/"><picture><source media="(prefers-color-scheme: light)" srcset="assets/readme/links-3-v2-light.svg"><img alt="Feature docs (docs/features/)" src="assets/readme/links-3-v2-dark.svg" width="32%"></picture></a>
</p>

<br>

<p>
<img alt="03 HOW IT WORKS. SPEC IN. GATES. CLIENTS OUT." src="assets/readme/divider-how-v2.svg" width="100%">
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/flow-v2-light.svg">
  <img alt="SPEC -&gt; LINT -&gt; GATE -&gt; CLIENTS. A pull request passes all four, or it does not merge." src="assets/readme/flow-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/how-v2-light.svg">
  <img alt="Lint: Spectral checks names, security and size limits. A selftest proves that each rule fires. Breaking: oasdiff on the APIs, eventcompat on the events, both against the PR base. Selftest: A fixture PR renames a required field. CI expects the gate to fail on it. Drift: CI generates the clients again and fails when the committed code differs" src="assets/readme/how-v2-dark.svg" width="100%">
</picture>
</p>

<br>

<p>
<img alt="04 USAGE. THE NAMES ARE THE RULES." src="assets/readme/divider-usage-v2.svg" width="100%">
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/api-v2-light.svg">
  <img alt="GET /healthz: The process runs. 200 with status ok. GET /readyz: One result per dependency. ok or degraded: 200. down: 503. pulse.target.status_changed.v1: Pulse saw a new readiness status of a target. llm.toolcall.executed.v1: Audit fact: the AI called one read-only MCP tool" src="assets/readme/api-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/naming-v2-light.svg">
  <img alt="dev.synthwerk.&lt;domain&gt;.&lt;entity&gt;.&lt;verb&gt;.v&lt;N&gt;: CloudEvents type. Verb in the past tense. evt.&lt;domain&gt;.&lt;entity&gt;.&lt;verb&gt;.v&lt;N&gt;: NATS subject. Stream EVT_&lt;DOMAIN&gt;. /v&lt;N&gt;/&lt;plural-noun&gt;: API path, kebab-case. operationId &lt;service&gt;.&lt;verb&gt;&lt;Noun&gt;. .v&lt;N+1&gt; or /v&lt;N+1&gt;/: The only way to make a breaking change" src="assets/readme/naming-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/reference-v2-light.svg">
  <img alt="Rules: Every name and its example: docs/naming.md. Guide: Layout, commands, CI and how to add an API: docs/developer-guide.md" src="assets/readme/reference-v2-dark.svg" width="100%">
</picture>
</p>

<p align="center">
<a href="openapi/_shared/health.yaml"><picture><source media="(prefers-color-scheme: light)" srcset="assets/readme/links-4-v2-light.svg"><img alt="Health spec (openapi/_shared/health.yaml)" src="assets/readme/links-4-v2-dark.svg" width="32%"></picture></a>
<a href="events/catalog.yaml"><picture><source media="(prefers-color-scheme: light)" srcset="assets/readme/links-5-v2-light.svg"><img alt="Event catalog (events/catalog.yaml)" src="assets/readme/links-5-v2-dark.svg" width="32%"></picture></a>
<a href="LICENSE"><picture><source media="(prefers-color-scheme: light)" srcset="assets/readme/links-6-v2-light.svg"><img alt="License (LICENSE)" src="assets/readme/links-6-v2-dark.svg" width="32%"></picture></a>
</p>

<br>

<p>
<img alt="05 STATUS. DRAFT. HONEST ABOUT IT." src="assets/readme/divider-status-v2.svg" width="100%">
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/status-v2-light.svg">
  <img alt="Health API: beta. Event envelope and catalog: beta. Breaking-change gate: working. Go and TS clients: generated, not published. Shared Go/Python kit: planned. Service APIs, MCP manifests: planned" src="assets/readme/status-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/now-v2-light.svg">
  <img alt="Now: Draft, milestone M1. Works: Health API, two events, the gates. Next: Publish, shared kit, pulse API" src="assets/readme/now-v2-dark.svg" width="100%">
</picture>
</p>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/checks-v2-light.svg">
  <img alt="just lint: Spectral, Biome, golangci-lint. just test: Go unit and contract tests, clients compile, selftests. just gen: Generate the clients. CI fails on drift" src="assets/readme/checks-v2-dark.svg" width="100%">
</picture>
</p>

<details>
<summary><picture><source media="(prefers-color-scheme: light)" srcset="assets/readme/links-7-v2-light.svg"><img alt="Plain text" src="assets/readme/links-7-v2-dark.svg" width="32%"></picture></summary>

```bash
just lint    # Spectral, Biome, golangci-lint
just test    # Go unit + contract tests, clients compile, selftests
just gen     # generate the clients; CI fails on drift
```

</details>

<p>
<picture>
  <source media="(prefers-color-scheme: light)" srcset="assets/readme/footer-v2-light.svg">
  <img alt="VM. studio / flagship · open source · look per vm-brand playbook · MIT (c) 2026 Velimir Mueller · -- EOF -- a rename is a breaking change --" src="assets/readme/footer-v2-dark.svg" width="100%">
</picture>
</p>
