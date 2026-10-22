# Solovey Node

This separate repository currently holds the R6 management-contract proof scope
only. It has no production Node implementation.

The Panel repository owns the versioned canonical semantic contract. Subsequent
normal PRs will pin the contract bundle and shared conformance fixtures here and
prove independent consumption with in-memory test stubs.

There is no agent, daemon, listener, remote transport, database, CoreRuntime,
installer, update service or Web UI. No Node product architecture beyond the
accepted versioned contracts is promised by this bootstrap.

This minimal initial commit establishes main in the previously empty repository.
All substantive R6 development after it uses normal branches, checks and PRs.
It makes no software-license decision and publishes no release.
