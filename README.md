# Solovey Node

This separate repository currently holds the R6 management-contract proof scope
only. It has no production Node implementation.

The Panel repository owns the canonical `solovey.management/v1` semantic contract.
`contracts/PIN.json` records source provenance and exact schema, generated-model,
vector and bundle digests. Checked-in files are copied by Panel's pin tool, never
maintained as a separate schema or registry. The module has no external dependency.

Run from the repository root with Go 1.26.10 and Node 24.16.0 available:

```text
node contracts/tools/generate.mjs --check
node scripts/verify-pin.mjs
go test ./...
go vet ./...
```

These checks work offline once the toolchains are installed. The public codec
executes shared shape, serialization, reason and digest vectors. The in-memory
`conformance` tests independently exercise trusted fixture grant binding,
revision admission, one-use consumption, replay and ambiguous-delivery retry.
The fixture map is not a credential store or an authority implementation.
Independent Component, removal, Change, Job, Health, Artifact and capability
consumers execute the same pinned state vectors as Panel's actual owner adapters.
Seven domain reference fixtures prove shared identity and owner revision scope;
they add no feature CRUD schema. The command consumer proves backup.restore only;
other catalog Actions require their own owner admission and capabilities.
Cancellation vectors model cooperative test signals and safe points. Current
Panel Job projections still report cancellation unsupported. Trusted fixture
verifier outcomes prove that artifact metadata alone cannot authorize use.
The dependency ratchet forbids Panel imports, external modules and Node runtime
entrypoints. It is a guard for this proof stage, not a production Node design.

There is no agent, daemon, listener, remote transport, database, CoreRuntime,
installer, update service or Web UI. No Node product architecture beyond the
accepted versioned contracts is promised by this bootstrap.

This minimal initial commit establishes main in the previously empty repository.
All substantive R6 development after it uses normal branches, checks and PRs.
It makes no software-license decision and publishes no release.
