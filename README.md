# Solovey Node

This independent repository remains an R6 management-contract conformance proof.
It has no production Node agent, daemon, listener, database or remote transport.

The sole editable `solovey.management/v1` schema, codec, generator and golden
vectors are maintained in
[solovey-contracts](https://github.com/MalenkiySolovey/solovey-contracts).
Node and Panel consume the same accepted immutable Go module, `v1.0.0`, with
source commit, tree and normal module checksums recorded in `contracts/PIN.json`.
Node has exactly this one external dependency and imports its public package,
without importing Panel implementation or maintaining a copied codec/schema.

With Go 1.26.10, Node 24.16.0 and `GOWORK=off` set:

```text
node scripts/verify-pin.mjs
node --test scripts/verify-pin.test.mjs
go mod verify
go test -race -count=1 ./...
go vet ./...
```

The verifier uses the actual Go resolver and rejects missing/wrong/mutable
sources, checksum/provenance drift, local replace/workspace/overlay substitution,
an editable schema or obsolete generator, and changed read-only fixtures.
An intentional accepted pin update may refresh only `vectors.json` and
`manifest.json` using `node scripts/verify-pin.mjs --sync-fixtures`.

All 115 unchanged golden cases have real independent consumers: 47 shape/digest/
reason cases, 23 trusted command-fixture cases, and 45 state/evidence/removal
cases. The producer owns its codec implementation tests. Node's fixture owner
models grant binding, one-use consumption, revision/replay and ambiguous delivery;
grant JSON alone authenticates nothing. Seven owner-reference fixtures retain
Host/resource/revision isolation. Capability UNKNOWN fails closed, SecretRef stays
masked, and artifact metadata alone does not confer trusted admission.

The dependency ratchet permits only the accepted public Contracts import plus
standard libraries, while preserving denial of Panel private/runtime/DB imports,
network/exec/SQL dependencies, production directories and daemon entrypoints.
The in-memory fixtures add no feature CRUD schema, token issuer, enrollment,
CoreRuntime, installer, update service or Web UI.

Contracts retains its original GPLv3 license and traceable Panel-source authorship.
This migration makes no new license decision for the original Node proof harness
and publishes no Node or Solovey UI product release. Rollback reverts the consumer
pin/import/fixture/retirement chain through an ordinary reviewed source revert;
published module tags and user data remain intact.
