# Read-only management conformance fixtures

`vectors.json` and `manifest.json` are exact derived fixtures from the immutable
Contracts module recorded in `contracts/PIN.json`. They support independent Node
tests without requiring network access inside the Go harness.

Schema, codec, generated types and generator are owned solely by
`MalenkiySolovey/solovey-contracts`. This directory contains no editable schema,
implementation or second generator. Propose semantic changes in the canonical
producer, qualify both consumers, and update the accepted module pin explicitly.

With `GOWORK=off`, `node scripts/verify-pin.mjs` resolves the real accepted module
and checks provenance, module sums, canonical bundle, source/generator and fixture
digests. `--sync-fixtures` refreshes only the two derived JSON fixtures after
that verification; it does not copy a schema or implementation back into Node.
