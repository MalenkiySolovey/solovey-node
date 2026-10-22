package conformance

import (
	"encoding/json"
	"strings"
	"testing"

	wire "github.com/MalenkiySolovey/solovey-node/contracts/management/v1"
)

// Only opaque owner revisions are modeled. There is no Inbound/Outbound/Route/
// DNS/User/Subscription implementation or generic feature CRUD contract here.
type scopedOwnerFixture struct {
	resource wire.ResourceRef
	desired  wire.Revision
}

func (owner *scopedOwnerFixture) compareAndPublish(scope wire.ResourceRef, expected, next wire.Revision) bool {
	if scope != owner.resource || expected != owner.desired {
		return false
	}
	owner.desired = next
	return true
}

func TestEveryDomainUsesTheSameHostResourceAndOwnerRevisionBoundary(t *testing.T) {
	owners := map[string]bool{}
	for _, raw := range vectorCases(t) {
		var vector struct {
			ID, Kind, Type string
			Document       json.RawMessage
		}
		if err := json.Unmarshal(raw, &vector); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(vector.ID, "owner-ref-") {
			continue
		}
		var scope wire.ResourceRef
		if err := wire.Decode("ResourceRef", vector.Document, &scope); err != nil {
			t.Fatal(err)
		}
		owners[scope.Owner] = true
		t.Run(scope.Owner, func(t *testing.T) {
			owner := scopedOwnerFixture{scope, "owner-revision:a"}
			panelObservation := owner.desired
			foreign := scope
			foreign.Host = wire.HostRef{Kind: "NODE", ID: "node:foreign"}
			if owner.compareAndPublish(foreign, panelObservation, "owner-revision:b") {
				t.Fatal("transport/foreign Host acquired ownership")
			}
			foreign = scope
			foreign.Owner = "foreign"
			if owner.compareAndPublish(foreign, panelObservation, "owner-revision:b") {
				t.Fatal("foreign owner acquired resource")
			}
			if !owner.compareAndPublish(scope, panelObservation, "owner-revision:b") {
				t.Fatal("exact owner-local revision failed")
			}
			panelObservation = "cached-panel-revision"
			if owner.desired != "owner-revision:b" || owner.compareAndPublish(scope, panelObservation, "owner-revision:c") {
				t.Fatal("Panel cache became desired-state authority")
			}
			if owner.compareAndPublish(scope, "owner-revision:a", "owner-revision:c") {
				t.Fatal("stale expected revision admitted")
			}
		})
	}
	for _, name := range []string{"inbounds", "outbounds", "routing", "dns", "vpn-users", "subscriptions", "component-lifecycle"} {
		if !owners[name] {
			t.Fatalf("missing owner scope proof %s", name)
		}
	}
}
