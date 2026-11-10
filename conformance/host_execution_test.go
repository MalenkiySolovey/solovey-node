package conformance

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	hostcore "github.com/MalenkiySolovey/solovey-host/coreruntime"
	"github.com/MalenkiySolovey/solovey-host/hostref"
)

// This adapter supplies deterministic provider effects and the owner's existing
// lease. Apply/verify/recovery and admission decisions execute only in Host.
type localSelectionAdapter struct {
	gate      chan struct{}
	allowed   bool
	admission hostcore.SelectionAdmission
	observed  hostcore.SelectionSnapshot
	writes    int
	effect    func(context.Context, string, int) error
}

func localSelectionFixture(provider string) (*localSelectionAdapter, hostcore.SelectionIntent) {
	host := hostref.Local()
	resource := hostcore.ResourceRef{Host: host, Owner: hostcore.SelectionOwner, Kind: hostcore.SelectionKind, ID: "choice"}
	identity := hostcore.Provider{ID: provider, Revision: strings.Repeat("a", 64)}
	f := &localSelectionAdapter{gate: make(chan struct{}, 1), allowed: true,
		admission: hostcore.SelectionAdmission{Host: host, Resource: resource, Provider: identity, Revision: "runtime-generation", Capability: hostcore.Available},
		observed:  hostcore.SelectionSnapshot{Host: host, Resource: resource, Provider: identity, Revision: "runtime-generation", Selectable: true, Selected: "direct", Members: []string{"direct", "second"}}}
	return f, hostcore.SelectionIntent{Host: host, Resource: resource, Provider: identity, ExpectedRevision: "runtime-generation", Member: "second"}
}
func (f *localSelectionAdapter) WithSelectionLease(ctx context.Context, _ hostcore.SelectionIntent, call func(context.Context, hostcore.SelectionLease) error) error {
	if !f.allowed {
		return hostcore.ErrSelectionScope
	}
	select {
	case f.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-f.gate }()
	return call(ctx, f)
}
func (f *localSelectionAdapter) Admission() hostcore.SelectionAdmission { return f.admission }
func (f *localSelectionAdapter) ReadSelection(ctx context.Context) (hostcore.SelectionSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return hostcore.SelectionSnapshot{}, err
	}
	result := f.observed
	result.Members = slices.Clone(result.Members)
	return result, nil
}
func (f *localSelectionAdapter) SelectMember(ctx context.Context, member string) error {
	f.writes++
	if f.effect != nil {
		return f.effect(ctx, member, f.writes)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	f.observed.Selected = member
	return nil
}

func TestSharedRuntimeSelectionExecutesBothProviderIntents(t *testing.T) {
	for _, provider := range []string{"sing-box", "shtorm-extended"} {
		t.Run(provider, func(t *testing.T) {
			owner, intent := localSelectionFixture(provider)
			result, err := hostcore.Select(context.Background(), intent, owner)
			if err != nil || result.Outcome != hostcore.SelectionVerified || result.Before != "direct" || result.Observed != "second" || result.Provider != intent.Provider || result.Health.Proof != "RUNTIME_SELECTION_VERIFIED" || result.Health.Source != "provider_api" {
				t.Fatalf("%+v %v", result, err)
			}
			retry, err := hostcore.Select(context.Background(), intent, owner)
			if err != nil || retry.ApplyAttempted || owner.writes != 1 {
				t.Fatal("idempotent retry dispatched", err)
			}
		})
	}
}

func TestSharedRuntimeSelectionRejectsForeignStaleAndUnauthorizedIntents(t *testing.T) {
	for _, kind := range []string{"host", "resource", "owner", "revision", "unauthorized", "unknown-capability", "unsupported-capability", "provider"} {
		t.Run(kind, func(t *testing.T) {
			owner, intent := localSelectionFixture("sing-box")
			switch kind {
			case "host":
				intent.Host.ID = "foreign"
			case "resource":
				intent.Resource.ID = "foreign"
			case "owner":
				intent.Resource.Owner = "dns"
			case "revision":
				intent.ExpectedRevision = "old-generation"
			case "unauthorized":
				owner.allowed = false
			case "unknown-capability":
				owner.admission.Capability = hostcore.Unknown
			case "unsupported-capability":
				owner.admission.Capability = hostcore.Unsupported
			case "provider":
				owner.admission.Provider.ID = "shtorm-extended"
			}
			result, err := hostcore.Select(context.Background(), intent, owner)
			if err == nil || owner.writes != 0 || result.Outcome != hostcore.SelectionRejected || result.Health.State != "UNKNOWN" {
				t.Fatalf("unsafe admission: %+v %v writes=%d", result, err, owner.writes)
			}
		})
	}
}

func TestSharedRuntimeSelectionVerifiesRollbackAndAmbiguousOutcomes(t *testing.T) {
	for _, mode := range []string{"failed-verification", "ambiguous-apply", "failed-rollback", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			owner, intent := localSelectionFixture("sing-box")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			owner.effect = func(ctx context.Context, member string, attempt int) error {
				if attempt == 1 {
					if mode == "failed-verification" {
						return nil
					}
					owner.observed.Selected = member
					if mode == "cancelled" {
						cancel()
						return ctx.Err()
					}
					return errors.New("deterministic lost provider acknowledgment")
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if mode == "failed-rollback" {
					return errors.New("deterministic restore failure")
				}
				owner.observed.Selected = member
				return nil
			}
			result, err := hostcore.Select(ctx, intent, owner)
			if err == nil || !result.ApplyAttempted || !result.RecoveryAttempted {
				t.Fatalf("failed apply became success: %+v %v", result, err)
			}
			if mode == "failed-rollback" {
				if result.Outcome != hostcore.SelectionAmbiguous || result.Health.State != "UNKNOWN" || !errors.Is(err, hostcore.ErrSelectionRecovery) {
					t.Fatal("ambiguous restore promoted", result, err)
				}
			} else if result.Outcome != hostcore.SelectionRestored || result.Observed != "direct" || result.Health.Proof != "RUNTIME_SELECTION_RESTORED" || owner.writes != 2 {
				t.Fatalf("unverified before-image: %+v %v", result, err)
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation", err)
			}
		})
	}
}

func TestSharedRuntimeSelectionConcurrentDuplicateUsesOneOwnerLease(t *testing.T) {
	owner, intent := localSelectionFixture("sing-box")
	entered, release := make(chan struct{}), make(chan struct{})
	owner.effect = func(_ context.Context, member string, _ int) error {
		close(entered)
		<-release
		owner.observed.Selected = member
		return nil
	}
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { _, err := hostcore.Select(context.Background(), intent, owner); first <- err }()
	<-entered
	go func() { _, err := hostcore.Select(context.Background(), intent, owner); second <- err }()
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if owner.writes != 1 {
		t.Fatal("duplicate concurrent apply", owner.writes)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := hostcore.Select(ctx, intent, owner); !errors.Is(err, context.Canceled) || owner.writes != 1 {
		t.Fatal("cancelled operation wrote", err)
	}
}
