package conformance

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	feature "github.com/MalenkiySolovey/solovey-component-observability-extra"
	"github.com/MalenkiySolovey/solovey-host/componenthost"
	"github.com/MalenkiySolovey/solovey-host/hostref"
	"github.com/MalenkiySolovey/solovey-host/observation"
	"github.com/MalenkiySolovey/solovey-host/scheduler"
	"github.com/robfig/cron/v3"
)

// A finite registration adapter, not a running cron service. Tests explicitly
// dispatch each bounded job; its cancellation/drain is the shared Host source.
type finiteComponentScheduler struct {
	mu   sync.Mutex
	next cron.EntryID
	jobs map[cron.EntryID]*scheduler.ManagedJob
	adds int
}

func newFiniteScheduler() *finiteComponentScheduler {
	return &finiteComponentScheduler{jobs: map[cron.EntryID]*scheduler.ManagedJob{}}
}
func (s *finiteComponentScheduler) AddJob(spec string, job cron.Job) (cron.EntryID, error) {
	if spec != "@every 2s" {
		return 0, errors.New("unapproved fixture schedule")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	s.adds++
	s.jobs[s.next] = scheduler.NewManagedJob(context.Background(), job, func(ctx context.Context) (context.Context, func(), error) { return ctx, func() {}, nil })
	return s.next, nil
}
func (*finiteComponentScheduler) Schedule(cron.Schedule, cron.Job) cron.EntryID { return 0 }
func (s *finiteComponentScheduler) RemoveJob(id cron.EntryID) {
	s.mu.Lock()
	job := s.jobs[id]
	s.mu.Unlock()
	if job != nil {
		job.CancelAndPrevent()
	}
}
func (s *finiteComponentScheduler) RemoveJobAndWait(ctx context.Context, id cron.EntryID) error {
	s.mu.Lock()
	job := s.jobs[id]
	s.mu.Unlock()
	if job == nil {
		return nil
	}
	job.CancelAndPrevent()
	if err := job.Wait(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.jobs, id)
	s.mu.Unlock()
	return nil
}
func (s *finiteComponentScheduler) first() *scheduler.ManagedJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[s.next]
}
func (s *finiteComponentScheduler) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.jobs) }

type lifecycleSampler struct {
	observations, cores atomic.Int32
	fail                bool
	entered, release    chan struct{}
}

func (s *lifecycleSampler) CurrentObservabilitySample() observation.ObservabilitySample {
	if s.entered != nil {
		close(s.entered)
		<-s.release
	}
	return observation.ObservabilitySample{DateTime: 1}
}
func (*lifecycleSampler) CurrentCoreSample() observation.CoreSample {
	return observation.CoreSample{DateTime: 1}
}
func (s *lifecycleSampler) RecordObservabilitySample(observation.ObservabilityBucket, observation.ObservabilitySample) error {
	s.observations.Add(1)
	if s.fail {
		return errors.New("sample owner unavailable")
	}
	return nil
}
func (s *lifecycleSampler) RecordCoreSample(observation.ObservabilityBucket, observation.CoreSample) error {
	s.cores.Add(1)
	return nil
}
func (*lifecycleSampler) HistoryForBucket(observation.ObservabilityBucket) ([]observation.ObservabilitySample, error) {
	return nil, nil
}
func (*lifecycleSampler) CoreHistoryForBucket(observation.ObservabilityBucket) ([]observation.CoreSample, error) {
	return nil, nil
}

func TestSharedComponentLifecycleUsesAcceptedFeatureAndAllReadinessCases(t *testing.T) {
	for _, state := range []string{"ABSENT", "INSTALLED_DISABLED", "ENABLED_UNSUPPORTED", "ENABLED_UNHEALTHY", "ENABLED_HEALTHY", "UNKNOWN"} {
		t.Run(state, func(t *testing.T) {
			schedule := newFiniteScheduler()
			sampler := &lifecycleSampler{fail: state == "ENABLED_UNHEALTHY"}
			var warnings atomic.Int32
			var active []componenthost.Entry
			if state != "ABSENT" && state != "INSTALLED_DISABLED" {
				descriptor := feature.Manifest()
				if descriptor.ID != "observability-extra" || descriptor.Validate() != nil {
					t.Fatal("wrong accepted component descriptor")
				}
				ports := componenthost.Ports{Host: hostref.Local(), Scheduler: schedule, ObservationSampler: sampler}
				if state == "ENABLED_UNSUPPORTED" {
					ports.Scheduler = nil
				}
				active = []componenthost.Entry{{ID: descriptor.ID, Lifecycle: feature.New(func(...any) { warnings.Add(1) }), Ports: ports}}
			}
			running, err := componenthost.Reconcile(context.Background(), nil, active, nil)
			if state == "ENABLED_UNSUPPORTED" {
				if err == nil || len(running) != 0 || schedule.count() != 0 {
					t.Fatal("missing lifecycle capability became active")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if state == "ABSENT" || state == "INSTALLED_DISABLED" {
				if len(running) != 0 || schedule.adds != 0 || sampler.observations.Load() != 0 {
					t.Fatal("inactive component has owned activity")
				}
				return
			}
			running, err = componenthost.Reconcile(context.Background(), running, active, nil)
			if err != nil || schedule.adds != 1 || schedule.count() != 1 {
				t.Fatal("repeated lifecycle duplicated registration", err)
			}
			job := schedule.first()
			if state == "UNKNOWN" {
				if sampler.observations.Load() != 0 {
					t.Fatal("Start fabricated sampling Health")
				}
			} else {
				job.Run()
				if state == "ENABLED_UNHEALTHY" {
					if warnings.Load() != 1 || sampler.cores.Load() != 0 {
						t.Fatal("failed sample became healthy")
					}
				} else if warnings.Load() != 0 || sampler.observations.Load() != 1 || sampler.cores.Load() != 1 {
					t.Fatal("actual sampling not verified")
				}
			}
			running, err = componenthost.Reconcile(context.Background(), running, nil, nil)
			if err != nil || len(running) != 0 || schedule.count() != 0 {
				t.Fatal("disable did not release drained resources", err)
			}
			before := sampler.observations.Load()
			job.Run()
			if sampler.observations.Load() != before {
				t.Fatal("disabled worker remained active")
			}
			if _, err := componenthost.Stop(context.Background(), running, nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSharedComponentLifecycleRetainsIncompleteDrainAndLeavesNoWorker(t *testing.T) {
	schedule := newFiniteScheduler()
	sampler := &lifecycleSampler{entered: make(chan struct{}), release: make(chan struct{})}
	entry := componenthost.Entry{ID: feature.Manifest().ID, Lifecycle: feature.New(nil), Ports: componenthost.Ports{Host: hostref.Local(), Scheduler: schedule, ObservationSampler: sampler}}
	running, err := componenthost.Reconcile(context.Background(), nil, []componenthost.Entry{entry}, nil)
	if err != nil {
		t.Fatal(err)
	}
	job := schedule.first()
	done := make(chan struct{})
	go func() { job.Run(); close(done) }()
	<-sampler.entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	running, err = componenthost.Stop(ctx, running, nil)
	if !errors.Is(err, context.Canceled) || len(running) != 1 || schedule.count() != 1 {
		t.Fatal("incomplete shutdown lost instance or registration", err)
	}
	close(sampler.release)
	<-done
	running, err = componenthost.Stop(context.Background(), running, nil)
	if err != nil || len(running) != 0 || schedule.count() != 0 {
		t.Fatal("drain retry failed", err)
	}
	before := sampler.observations.Load()
	job.Run()
	if sampler.observations.Load() != before {
		t.Fatal("orphan worker after stop-and-drain")
	}
}
