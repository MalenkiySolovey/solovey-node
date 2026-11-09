package conformance

import (
	"encoding/json"
	"testing"

	wire "github.com/MalenkiySolovey/solovey-contracts/management/v1"
)

// Exercise the public pinned module as an independent consumer. The producer
// owns codec implementation tests; Node must still execute these 47 golden
// documents/digests/reasons after its copied codec test package is retired.
func TestSharedSchemaDigestAndReasonVectors(t *testing.T) {
	count := 0
	for _, raw := range vectorCases(t) {
		var vector struct {
			ID, Kind, Type                string
			Document                      json.RawMessage
			Valid, Equal, Accepted        bool
			First, Second                 wire.Command
			ExpectedFirst, ExpectedSecond string
			Result                        wire.CommandResult
			Reason                        wire.ReasonCode
		}
		if err := json.Unmarshal(raw, &vector); err != nil {
			t.Fatal(err)
		}
		if vector.Kind != "document" && vector.Kind != "digest" && vector.Kind != "reason" {
			continue
		}
		count++
		t.Run(vector.ID, func(t *testing.T) {
			switch vector.Kind {
			case "document":
				var output json.RawMessage
				err := wire.Decode(vector.Type, vector.Document, &output)
				if (err == nil) != vector.Valid {
					t.Fatalf("valid=%v: %v", vector.Valid, err)
				}
				if err == nil && wire.ValidateDocument(vector.Type, output) != nil {
					t.Fatal("decoded document failed round-trip validation")
				}
			case "digest":
				first, err := wire.CommandDigest(vector.First)
				if err != nil {
					t.Fatal(err)
				}
				second, err := wire.CommandDigest(vector.Second)
				if err != nil || (first == second) != vector.Equal || first != vector.ExpectedFirst || second != vector.ExpectedSecond {
					t.Fatalf("canonical digest differs from golden: %s / %s: %v", first, second, err)
				}
			case "reason":
				if wire.Accepted(vector.Result) != vector.Accepted || wire.EffectiveReason(vector.Result.Reason) != vector.Reason {
					t.Fatal("reason evolution changed success semantics")
				}
			}
		})
	}
	if count != 47 {
		t.Fatalf("executed %d schema/digest/reason cases, expected 47", count)
	}
}
