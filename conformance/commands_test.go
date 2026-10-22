package conformance

import (
	"encoding/json"
	"os"
	"testing"

	wire "github.com/MalenkiySolovey/solovey-node/contracts/management/v1"
)

// Everything in this file is an in-memory conformance fixture, never a daemon,
// authority, command endpoint, feature owner implementation or persistence API.
type commandCase struct {
	ID, Kind                                                     string
	Now                                                          int64
	Host                                                         wire.HostRef
	Resource                                                     wire.ResourceRef
	Revision, NextRevision                                       wire.Revision
	Generation                                                   wire.Generation
	ActorRef, AuthorityRef                                       string
	AuthorizationGeneration, CredentialGeneration, MFAGeneration wire.Generation
	Grants                                                       []wire.ManagementGrant
	Steps                                                        []struct {
		Command      wire.Command
		DropResponse bool
		Expected     struct {
			Reason    wire.ReasonCode
			Mutations int
			Replay    bool
		}
	}
}

func vectorCases(t *testing.T) []json.RawMessage {
	t.Helper()
	data, err := os.ReadFile("../contracts/management/v1/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var set struct {
		Version string
		Cases   []json.RawMessage
	}
	if err := json.Unmarshal(data, &set); err != nil || set.Version != "solovey.management.conformance/v1" {
		t.Fatalf("vectors: %v", err)
	}
	return set.Cases
}

type recorded struct {
	digest string
	result wire.CommandResult
}
type fixtureOwner struct {
	context   commandCase
	revision  wire.Revision
	mutations int
	used      map[string]bool
	records   map[string]recorded
}

func newFixtureOwner(input commandCase) *fixtureOwner {
	return &fixtureOwner{context: input, revision: input.Revision, used: map[string]bool{}, records: map[string]recorded{}}
}

// Grants here are prevalidated trusted test inputs. Decoding ManagementGrant
// JSON alone is never authentication. A future authority must verify it first.
func (owner *fixtureOwner) admit(command wire.Command) wire.CommandResult {
	result := wire.CommandResult{ContractVersion: wire.Version, CommandID: command.CommandID, Host: command.Host, Resource: command.Resource,
		Status: "REJECTED", Reason: "INVALID_INPUT", ObservedRevision: owner.revision, Generation: owner.context.Generation, CorrelationID: command.CorrelationID}
	reject := func(reason wire.ReasonCode) wire.CommandResult { result.Reason = reason; return result }
	if err := wire.ValidateCommand(command); err != nil {
		reason := wire.ReasonCode(err.Error())
		if !wire.KnownReason(reason) {
			reason = "INVALID_INPUT"
		}
		return reject(reason)
	}
	if command.Host != owner.context.Host || command.Resource != owner.context.Resource {
		return reject("PERMISSION_DENIED")
	}
	if command.Generation != owner.context.Generation {
		return reject("STALE_STATE")
	}
	var grant *wire.ManagementGrant
	for i := range owner.context.Grants {
		if owner.context.Grants[i].GrantID == command.GrantRef {
			grant = &owner.context.Grants[i]
			break
		}
	}
	if grant == nil || grant.ExpiresAt <= owner.context.Now || owner.used[grant.GrantID] || owner.used[grant.Nonce] {
		return reject("APPROVAL_REQUIRED")
	}
	raw, err := json.Marshal(grant)
	if err != nil || wire.ValidateDocument("ManagementGrant", raw) != nil {
		return reject("APPROVAL_REQUIRED")
	}
	if grant.AuthorityRef != owner.context.AuthorityRef || grant.AuthorizationGeneration != owner.context.AuthorizationGeneration ||
		grant.CredentialGeneration != owner.context.CredentialGeneration || grant.MfaGeneration != owner.context.MFAGeneration {
		return reject("AUTHORITY_UNAVAILABLE")
	}
	descriptor, _ := wire.LookupAction(command.ActionID)
	if grant.ActorRef != owner.context.ActorRef || grant.ActionID != command.ActionID || grant.Permission != descriptor.Permission ||
		grant.Host != command.Host || grant.Resource != command.Resource || grant.InputDigest != command.InputDigest || !grant.OneUse {
		return reject("PERMISSION_DENIED")
	}
	owner.used[grant.GrantID], owner.used[grant.Nonce] = true, true
	digest, err := wire.CommandDigest(command)
	if err != nil {
		return reject("INVALID_INPUT")
	}
	if previous, exists := owner.records[command.CommandID]; exists {
		if previous.digest != digest {
			return reject("RESOURCE_CONFLICT")
		}
		result = previous.result
		result.Reason, result.Replay = "ALREADY_EXISTS", true
		result.CorrelationID = command.CorrelationID
		return result
	}
	if command.ExpectedRevision == nil || *command.ExpectedRevision != owner.revision {
		return reject("REVISION_CONFLICT")
	}
	// Owner-local fixture state is the only desired-state authority. A caller's
	// projection or command never becomes a second authoritative feature store.
	owner.revision, owner.mutations = owner.context.NextRevision, owner.mutations+1
	result.ObservedRevision, result.Status, result.Reason = owner.revision, "ACCEPTED", "ACCEPTED"
	changeID, jobID := "change:"+command.CommandID, "job:"+command.CommandID
	result.ChangeID, result.JobID = &changeID, &jobID
	owner.records[command.CommandID] = recorded{digest, result}
	return result
}

func runCommandCase(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var vector commandCase
	if err := json.Unmarshal(raw, &vector); err != nil {
		t.Fatal(err)
	}
	owner := newFixtureOwner(vector)
	var first *wire.CommandResult
	for _, step := range vector.Steps {
		result := owner.admit(step.Command)
		if result.Reason != step.Expected.Reason || result.Replay != step.Expected.Replay || owner.mutations != step.Expected.Mutations {
			t.Fatalf("result=%+v mutations=%d expected=%+v", result, owner.mutations, step.Expected)
		}
		encoded, err := json.Marshal(result)
		if err != nil || wire.ValidateDocument("CommandResult", encoded) != nil {
			t.Fatalf("invalid result: %s %v", encoded, err)
		}
		if result.Replay && (first == nil || *result.ChangeID != *first.ChangeID || *result.JobID != *first.JobID || result.ObservedRevision != first.ObservedRevision) {
			t.Fatal("retry changed durable result identity")
		}
		if result.Status == "ACCEPTED" && !wire.Accepted(result) {
			t.Fatal("accepted result lost its semantic reason")
		}
		if first == nil && result.Status == "ACCEPTED" {
			copy := result
			first = &copy
		}
		// dropResponse intentionally affects only delivery. The owner has already
		// recorded acceptance, so the next delivery must replay the same result.
		if step.DropResponse && result.Status != "ACCEPTED" {
			t.Fatal("ambiguous-delivery fixture did not accept")
		}
	}
}

func TestSharedCommandVectors(t *testing.T) {
	count := 0
	for _, raw := range vectorCases(t) {
		var header struct{ ID, Kind string }
		if err := json.Unmarshal(raw, &header); err != nil {
			t.Fatal(err)
		}
		if header.Kind != "command" {
			continue
		}
		count++
		t.Run(header.ID, func(t *testing.T) { runCommandCase(t, raw) })
	}
	if count < 23 {
		t.Fatalf("command coverage missing: %d", count)
	}
}
