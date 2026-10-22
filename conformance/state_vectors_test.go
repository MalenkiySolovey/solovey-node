package conformance

import (
	"encoding/json"
	"testing"

	wire "github.com/MalenkiySolovey/solovey-node/contracts/management/v1"
)

// These are proof fixtures. No feature implementation, worker, process, storage
// or transport is constructed. Their only input is the pinned shared vector set.
func fixtureEvidenceSatisfies(e wire.HealthEvidence, resource wire.ResourceRef, revision wire.Revision, at int64, generation wire.Generation) bool {
	raw, err := json.Marshal(e)
	if err != nil || wire.ValidateDocument("HealthEvidence", raw) != nil {
		return false
	}
	// The accepted R3 checker recognizes digest revisions. An unfamiliar owner
	// revision can be described on the wire but cannot manufacture this proof.
	revisionJSON, err := json.Marshal(e.ObservedRevision)
	if err != nil || wire.ValidateDocument("Digest", revisionJSON) != nil {
		return false
	}
	if e.Resource != resource || e.ProofKind != "PROBE_OK" || e.Source != "fixture" || e.ObservedRevision != revision {
		return false
	}
	if e.ObservedAt > at || at-e.ObservedAt > e.MaxAgeSeconds {
		return false
	}
	return generation == "" || e.RuntimeGeneration != nil && *e.RuntimeGeneration == generation
}

func fixtureArtifactAdmission(identity wire.ArtifactIdentity, verifiedByTrustedFixture bool) bool {
	if !verifiedByTrustedFixture {
		return false
	}
	raw, err := json.Marshal(identity)
	return err == nil && wire.ValidateDocument("ArtifactIdentity", raw) == nil
}

func assertProjection(t *testing.T, name string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip json.RawMessage
	if err := wire.Decode(name, raw, &roundTrip); err != nil {
		t.Fatalf("%s: %s: %v", name, raw, err)
	}
	if err := wire.ValidateDocument(name, roundTrip); err != nil {
		t.Fatal(err)
	}
}

func TestAllSharedVectorKindsHaveIndependentConsumers(t *testing.T) {
	counts := map[string]int{}
	for _, raw := range vectorCases(t) {
		var v struct {
			ID, Kind                                              string
			Installed, Enabled, Available                         bool
			Lifecycle, Health                                     string
			SourceStatus, FactCode, ExpectedStatus                string
			Expected                                              json.RawMessage
			Evidence                                              wire.HealthEvidence
			Now                                                   int64
			ExpectedRevision                                      wire.Revision
			ExpectedGeneration, Generation, ResultGeneration      wire.Generation
			Valid, Accepted, Critical                             bool
			Mode, Retention                                       string
			Current, Running                                      bool
			Known                                                 *bool
			Reference                                             wire.OperationRef
			Initial, ExpectedAfterRequest, ExpectedAfterSafePoint string
			Artifact                                              wire.ArtifactIdentity
			Status, Value, Revision, Phase, OwnerPhase            string
			Outcome                                               wire.Outcome
			RollbackVerified, RecoveryRequired                    bool
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		counts[v.Kind]++
		t.Run(v.ID, func(t *testing.T) {
			switch v.Kind {
			case "document", "digest", "reason", "command":
				// The pinned public codec suite and TestSharedCommandVectors run these.
			case "component":
				component := wire.Component{Host: wire.HostRef{Kind: "LOCAL", ID: "local"}, Manifest: wire.ComponentManifest{ComponentID: "test-owner", Version: "1.0.0", Actions: []string{}}, State: "UNKNOWN", Reason: "UNKNOWN", LifecyclePhase: &v.Lifecycle}
				scope := wire.ResourceRef{Host: component.Host, Owner: component.Manifest.ComponentID, ResourceID: "resource:fixture"}
				at := v.Now
				if at == 0 {
					at = v.Evidence.ObservedAt
				}
				qualified := fixtureEvidenceSatisfies(v.Evidence, scope, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", at, "")
				switch {
				case !v.Installed:
					component.State, component.Reason = "ABSENT", "ABSENT"
				case !v.Enabled:
					component.State, component.Reason = "INSTALLED_DISABLED", "DISABLED"
				case !v.Available:
					component.State, component.Reason = "ENABLED_UNSUPPORTED", "UNSUPPORTED"
				case v.Health == "PASS" && qualified:
					component.State, component.Reason, component.EvidenceRef = "ENABLED_HEALTHY", "ACCEPTED", &v.Evidence.EvidenceRef
				case v.Health == "FAIL" && qualified:
					component.State, component.Reason = "ENABLED_UNHEALTHY", "DEPENDENCY_UNAVAILABLE"
				}
				var expected string
				if err := json.Unmarshal(v.Expected, &expected); err != nil {
					t.Fatal(err)
				}
				if component.State != expected {
					t.Fatalf("state=%s expected=%s", component.State, expected)
				}
				assertProjection(t, "Component", component)
			case "health":
				scope := wire.ResourceRef{Host: wire.HostRef{Kind: "LOCAL", ID: "local"}, Owner: "test-owner", ResourceID: "resource:fixture"}
				valid := fixtureEvidenceSatisfies(v.Evidence, scope, v.ExpectedRevision, v.Now, v.ExpectedGeneration)
				qualified := valid
				valid = valid && (v.SourceStatus == "" || v.SourceStatus == "ok")
				if valid != v.Valid {
					t.Fatalf("valid=%v expected=%v", valid, v.Valid)
				}
				result := wire.Health{Resource: scope, Status: "UNKNOWN", Reason: "STALE_STATE"}
				if valid {
					result.Status, result.Reason, result.Evidence = "PASS", "ACCEPTED", &v.Evidence
				}
				if v.SourceStatus == "degraded" && qualified && v.FactCode == "fixture_failed" {
					result.Status, result.Reason, result.Evidence = "FAIL", "DEPENDENCY_UNAVAILABLE", &v.Evidence
				}
				if v.ExpectedStatus != "" && result.Status != v.ExpectedStatus {
					t.Fatalf("status=%s expected=%s", result.Status, v.ExpectedStatus)
				}
				assertProjection(t, "Health", result)
			case "removal":
				// A current supervisor that never created an instance can admit;
				// retained/failed/replaced incarnations cannot discard ownership.
				admitted := v.Current && !v.Running && v.Retention != "RETAINED" && v.Phase != "START_FAILED"
				if admitted != v.Accepted {
					t.Fatal("component lifecycle removal fence differs")
				}
			case "job":
				// Cancellation is an owner-cooperative fixture signal. Requesting
				// it cannot publish completion or undo a previously accepted result.
				phase := v.Initial
				terminal := phase == "SUCCEEDED"
				after := phase
				if !terminal {
					after = "CANCEL_REQUESTED"
					if v.Critical {
						after = "DEFERRED_UNTIL_SAFE_POINT"
					}
				}
				if after != v.ExpectedAfterRequest {
					t.Fatal(after)
				}
				if !terminal {
					phase = "CANCELLED"
				}
				if phase != v.ExpectedAfterSafePoint {
					t.Fatal(phase)
				}
				outcome := wire.Outcome("FAILED")
				if terminal {
					outcome = "COMMITTED"
				}
				job := wire.Job{Reference: v.Reference, JobID: "job:fixture", Revision: "1", Phase: phase, Outcome: outcome, CancelSupported: false, CriticalSection: v.Critical}
				assertProjection(t, "Job", job)
			case "lateResult":
				current := "current-owner-truth"
				observed := current
				accepted := v.Generation == v.ResultGeneration
				if accepted {
					observed = "late-result"
				}
				if accepted != v.Valid || !accepted && observed != current {
					t.Fatal("late old-generation result replaced owner truth")
				}
			case "artifact":
				// The fixture verifier has three explicit trusted outcomes; wire
				// metadata itself is never the verifier or a bearer credential.
				trusted := v.Status == "VERIFIED"
				if fixtureArtifactAdmission(v.Artifact, trusted) != v.Accepted {
					t.Fatalf("trust=%s", v.Status)
				}
				if fixtureArtifactAdmission(v.Artifact, false) {
					t.Fatal("identity metadata manufactured trust")
				}
			case "capability":
				if v.Known != nil && !*v.Known {
					v.Value = "unknown"
				}
				capability := wire.Capability{Host: wire.HostRef{Kind: "LOCAL", ID: "local"}, Owner: "test-owner", ID: "graceful-drain", Value: v.Value, Revision: "a", Source: "fixture"}
				assertProjection(t, "Capability", capability)
				if (capability.Value == "yes") != v.Accepted {
					t.Fatal("unknown critical capability admitted")
				}
			case "change":
				change := wire.Change{Reference: v.Reference, Revision: wire.Generation(v.Revision), Phase: v.Phase, OwnerPhase: v.OwnerPhase, Outcome: v.Outcome, RollbackVerified: v.RollbackVerified, RecoveryRequired: v.RecoveryRequired}
				assertProjection(t, "Change", change)
				job := wire.Job{Reference: change.Reference, JobID: "job:fixture", Revision: change.Revision, Phase: change.OwnerPhase, Outcome: change.Outcome, RecoveryRequired: change.RecoveryRequired}
				assertProjection(t, "Job", job)
				if change.Outcome == "ROLLED_BACK" && !change.RollbackVerified || change.Outcome == "RECOVERY_REQUIRED" && !change.RecoveryRequired {
					t.Fatal("recovery truth lost")
				}
			default:
				t.Fatalf("unexecuted shared vector kind %q", v.Kind)
			}
		})
	}
	for _, kind := range []string{"document", "digest", "reason", "command", "component", "health", "job", "lateResult", "artifact", "capability", "change", "removal"} {
		if counts[kind] == 0 {
			t.Fatalf("missing kind %s", kind)
		}
	}
	t.Logf("all %d shared vector cases accounted for; semantic consumers executed separately from the pinned codec", len(vectorCases(t)))
}
