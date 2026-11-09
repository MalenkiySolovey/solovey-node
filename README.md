# Solovey Node

This repository contains independent conformance and shared-library qualification for Solovey Node integrations. It exercises management commands, owner references, evidence and host-local algorithms using deterministic fixtures. It currently provides a test harness without a production agent, listener, database or remote transport.

## Shared sources

- [Solovey Contracts](https://github.com/MalenkiySolovey/solovey-contracts) owns the `solovey.management/v1` schema, codec, generator and 115 golden management cases.
- [Solovey Host](https://github.com/MalenkiySolovey/solovey-host) provides Local Host/Network Identity algorithms, declared component descriptors and capabilities, scheduled-job cancellation/drain and scoped secret/operation-reference values. Neutral audit and observation projections retain their existing JSON representation.

Immutable module versions, source commits and normal Go checksums are recorded in `contracts/PIN.json` and `host/PIN.json`. The harness calls those public packages directly. Read-only fixtures are verified against the resolved modules; no Panel private implementation is imported.

## Development

Use Go 1.26.10, Node 24.16.0 and `GOWORK=off`:

```text
node scripts/verify-pin.mjs
node scripts/verify-host-pin.mjs
node --test scripts/verify-pin.test.mjs scripts/verify-host-pin.test.mjs
go mod verify
go test -race -count=1 ./...
go vet ./...
```

The management fixtures cover strict shapes, digest validation, grant/revision binding, replay, ambiguous delivery, owner scope, masked SecretRef and unavailable capabilities. Network fixtures cover canonical JSON, stale and future evidence, invalid authority, oversized collections, unknown facts, address scopes and revision determinism. Additional public-library tests exercise real cancellation/drain with injected owner admission, declared-port rejection, owner/intent/operation denial and 13 canonical neutral values.

Dependency checks allow only the accepted public packages and standard libraries needed by these tests. They reject Panel/private imports, network execution, SQL, shell commands, production directories and daemon entrypoints. `net/netip` is used only for address parsing and classification.

The shared scheduled-job contract uses the existing `github.com/robfig/cron/v3` v3.0.1 dependency. The harness creates no scheduler or recurring work; its tests run finite jobs with explicit cancellation and deadlines.

The pin verifiers reject mutable versions, local replacement/workspace/overlay substitution, checksum drift, changed derived fixtures and duplicate editable implementations. Intentional accepted pin updates can refresh derived fixtures using each verifier's `--sync-fixtures` option.

## Licensing

Contracts and Host retain their original GPLv3 license and source attribution. This repository's original conformance harness retains its existing licensing status. Module consumption does not publish a Node or Solovey UI product release.
