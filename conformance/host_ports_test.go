package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	managementv1 "github.com/MalenkiySolovey/solovey-contracts/management/v1"
	"github.com/MalenkiySolovey/solovey-host/componenthost/manifest"
	"github.com/MalenkiySolovey/solovey-host/hostref"
	"github.com/MalenkiySolovey/solovey-host/scheduler"
	"github.com/MalenkiySolovey/solovey-host/secretref"
)

func TestPublicReferencesPreserveManagementValueShapes(t *testing.T) {
	local := hostref.Local()
	reference := secretref.NewReference("telegram", "telegramBotToken")
	for _, pair := range [][2]any{
		{local, managementv1.HostRef{Kind: local.Kind, ID: local.ID}},
		{reference, managementv1.SecretRef{Owner: reference.Owner, ID: reference.ID, Policy: reference.Policy, Projection: reference.Projection}},
	} {
		got, err := json.Marshal(pair[0])
		if err != nil {
			t.Fatal(err)
		}
		want, err := json.Marshal(pair[1])
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("local identity/reference value diverged from the canonical management representation")
		}
	}
}

func TestScopedSecretReferencesRejectForeignAuthority(t *testing.T) {
	base := secretref.Access{Reference: secretref.NewReference("telegram", "telegramBotToken"), Actor: secretref.Actor{Component: "telegram"}, Operation: secretref.UseForOwner, IntentID: "fixture-intent"}
	if base.Validate() != nil {
		t.Fatal("valid owner-use reference denied")
	}
	for _, change := range []func(*secretref.Access){
		func(a *secretref.Access) { a.Actor.Component = "foreign" },
		func(a *secretref.Access) { a.Operation = "REVEAL" },
		func(a *secretref.Access) { a.IntentID = "" },
		func(a *secretref.Access) { a.IntentID = "untrusted intent" },
		func(a *secretref.Access) { a.Reference.Projection = "RAW" },
		func(a *secretref.Access) { a.Reference.Policy = "ARBITRARY" },
	} {
		candidate := base
		change(&candidate)
		if candidate.Validate() != secretref.ErrDenied || candidate.View(true, "").Present {
			t.Fatal("foreign owner/intent/operation gained a secret view")
		}
	}
}

func TestDeclaredLifecyclePortsRejectBroadAuthority(t *testing.T) {
	valid := manifest.HostAPI{Version: manifest.HostAPIV1, Lifecycle: []string{manifest.PortScheduler, manifest.PortAuditRecord}, API: []string{manifest.PortHTTPRequest}, DropData: []string{manifest.PortSettingsDrop}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, contract := range []manifest.HostAPI{
		{Version: "future"},
		{Version: manifest.HostAPIV1, Lifecycle: []string{"runtime"}},
		{Version: manifest.HostAPIV1, API: []string{"database"}},
		{Version: manifest.HostAPIV1, API: []string{manifest.PortScheduler}},
		{Version: manifest.HostAPIV1, Lifecycle: []string{manifest.PortScheduler, manifest.PortScheduler}},
	} {
		if contract.Validate() == nil {
			t.Fatal("undeclared or broad authority was admitted")
		}
	}
}

func TestInstalledDescriptorUsesCanonicalValidation(t *testing.T) {
	data := []byte(`{"id":"fixture","name":"Fixture","version":"1.0.0","since":"1.0.0","delivery":"in-process","defaultEnabled":false,"hostAPI":{"version":"solovey.component-host/v1","lifecycle":["scheduler"]}}`)
	descriptor, err := manifest.FromJSON(data)
	if err != nil || descriptor.ID != "fixture" || descriptor.DefaultEnabled || descriptor.HostAPI == nil {
		t.Fatal("public installed descriptor changed", err)
	}
	for _, malformed := range [][]byte{
		bytes.Replace(data, []byte(`"defaultEnabled":false`), []byte(`"defaultEnabled":false,"untrusted":true`), 1),
		bytes.Replace(data, []byte(`"scheduler"`), []byte(`"runtime"`), 1),
		append(append([]byte{}, data...), []byte(` {}`)...),
	} {
		if _, err := manifest.FromJSON(malformed); err == nil {
			t.Fatal("untrusted installed descriptor admitted")
		}
	}
}

type hostContextJob struct {
	started chan struct{}
	finish  <-chan struct{}
}

func (hostContextJob) Run() { panic("context-aware scheduled job lost its context") }

func (job hostContextJob) RunContext(ctx context.Context) {
	close(job.started)
	if job.finish == nil {
		<-ctx.Done()
	} else {
		<-job.finish
	}
}

func TestSharedManagedJobCancelsAndDrainsOwnerAdmission(t *testing.T) {
	var admitted, released atomic.Int32
	job := hostContextJob{started: make(chan struct{})}
	managed := scheduler.NewManagedJob(t.Context(), job, func(ctx context.Context) (context.Context, func(), error) {
		admitted.Add(1)
		return ctx, func() { released.Add(1) }, nil
	})
	go managed.Run()
	select {
	case <-job.started:
	case <-time.After(3 * time.Second):
		t.Fatal("shared managed job did not execute")
	}
	managed.CancelAndPrevent()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := managed.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	managed.Run()
	if admitted.Load() != 1 || released.Load() != 1 {
		t.Fatal("cancelled job restarted or released admission before drain")
	}
}

func TestSharedManagedJobDeadlineRetainsOwnerOperation(t *testing.T) {
	finish := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(finish) }) }
	t.Cleanup(release)
	var released atomic.Bool
	job := hostContextJob{started: make(chan struct{}), finish: finish}
	managed := scheduler.NewManagedJob(t.Context(), job, func(ctx context.Context) (context.Context, func(), error) {
		return ctx, func() { released.Store(true) }, nil
	})
	go managed.Run()
	select {
	case <-job.started:
	case <-time.After(3 * time.Second):
		t.Fatal("shared managed job did not execute")
	}
	managed.CancelAndPrevent()
	short, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	if err := managed.Wait(short); !errors.Is(err, context.DeadlineExceeded) || released.Load() {
		t.Fatal("deadline invented owner completion")
	}
	release()
	ctx, done := context.WithTimeout(t.Context(), 3*time.Second)
	defer done()
	if err := managed.Wait(ctx); err != nil || !released.Load() {
		t.Fatal("retained owner operation could not drain on retry")
	}
}
