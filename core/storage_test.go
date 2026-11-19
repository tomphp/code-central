package core_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/usage"
)

// The Storage tab's read (Spec code-central-1032): where the Store is and
// how big, each Project's records, the process's memory and the services
// registered on the Store, as they are now.

// storageWorld is acme, with two Specs, a Ticket and Usage on two days,
// and beta with none, on one Store with two services registered: this
// one on laptop, and another on desktop.
func storageWorld(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	f.core.SetUsage(usage.NewMemory(f.clock, time.UTC, nil))
	f.project(t, "acme")
	f.project(t, "beta")
	spec := f.spec(t, "acme", "Pick a colour")
	f.spec(t, "acme", "Pick a font")
	f.ticket(t, "acme", api.CreateTicketInput{Title: "Paint it", Spec: api.Ref(spec.ID)})
	f.core.RecordCall("acme", usage.Call{Op: "get_ticket"})
	f.clock.Advance(24 * time.Hour)
	f.core.RecordCall("acme", usage.Call{Op: "find_tickets"})
	f.core.RecordCall("acme", usage.Call{Op: "get_spec"})

	f.core.SetService(core.Service{ID: "a", Host: "laptop", Version: "v1.2.0"})
	desktop := core.New(f.store, f.clock, &ids.Sequence{})
	desktop.SetService(core.Service{ID: "b", Host: "desktop", Version: "v1.3.0"})
	for _, c := range []*core.Core{f.core, desktop} {
		if err := c.Register(ctx); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	return f
}

func getStorage(t *testing.T, f fixture, in api.GetStorageInput) api.StorageView {
	t.Helper()
	v, err := f.core.GetStorage(ctx, in)
	if err != nil {
		t.Fatalf("get_storage: %v", err)
	}
	return v
}

func TestStorageShowsTheStoreEachProjectsCountsTheProcessAndTheServices(t *testing.T) {
	f := storageWorld(t)
	v := getStorage(t, f, api.GetStorageInput{})

	if v.Kind != "memory" || v.Path != "" || v.Host != "" {
		t.Errorf("the fake shows as %q at %q%q, want memory, nowhere", v.Kind, v.Path, v.Host)
	}
	if v.Size <= 0 {
		t.Errorf("size %d, want the records' size", v.Size)
	}
	want := []api.StorageProject{
		{Key: "acme", Name: "acme", Specs: 2, Tickets: 1, UsageDays: 2},
		{Key: "beta", Name: "beta"},
	}
	if !slices.Equal(v.Projects, want) {
		t.Errorf("projects %+v, want %+v", v.Projects, want)
	}
	if v.Memory.Heap <= 0 {
		t.Errorf("heap %d, want the Go heap in use", v.Memory.Heap)
	}
	var services []string
	for _, s := range v.Services {
		services = append(services, s.Host+" "+s.Version)
		if !s.Renewed.Equal(t0.Add(24 * time.Hour)) {
			t.Errorf("%s renewed %v, want when it registered", s.Host, s.Renewed)
		}
	}
	if got := []string{"desktop v1.3.0", "laptop v1.2.0"}; !slices.Equal(services, got) {
		t.Errorf("services %v, want %v", services, got)
	}
}

// A service that stopped renewing its Registration is gone from the
// list once it lapses.
func TestStorageLeavesOutAServiceWhoseRegistrationLapsed(t *testing.T) {
	f := storageWorld(t)
	f.clock.Advance(store.ServiceLife)
	if err := f.core.Register(ctx); err != nil {
		t.Fatal(err)
	}
	v := getStorage(t, f, api.GetStorageInput{})
	if len(v.Services) != 1 || v.Services[0].Host != "laptop" {
		t.Errorf("services %+v, want laptop alone", v.Services)
	}
}

// The scope narrows the counts to one Project; the Store, the process's
// memory and the services are whole whatever the scope.
func TestStorageScopedToAProjectCountsItAlone(t *testing.T) {
	f := storageWorld(t)
	all := getStorage(t, f, api.GetStorageInput{})
	v := getStorage(t, f, api.GetStorageInput{Project: "beta"})
	if want := []api.StorageProject{{Key: "beta", Name: "beta"}}; !slices.Equal(v.Projects, want) {
		t.Errorf("projects %+v, want %+v", v.Projects, want)
	}
	if v.Kind != all.Kind || v.Size != all.Size || len(v.Services) != 2 || v.Memory.Heap <= 0 {
		t.Errorf("scoped to beta: %+v, want the Store, memory and services whole", v)
	}
}

func TestStorageOfAnUnknownProjectIsNotFound(t *testing.T) {
	f := storageWorld(t)
	_, err := f.core.GetStorage(ctx, api.GetStorageInput{Project: "gamma"})
	var e *api.Error
	if !errors.As(err, &e) || e.Code != api.NotFound {
		t.Fatalf("get_storage of gamma: %v, want not_found", err)
	}
}

// postgresAt is a Store that says it is the Postgres Store at url.
type postgresAt struct {
	store.Store
	location store.Location
}

func (s postgresAt) Location() store.Location { return s.location }

// A Postgres Store shows as its host, port and database, and nothing of
// the credential its URL holds.
func TestStorageOfAPostgresStoreShowsItsAddressAndNoCredential(t *testing.T) {
	l, err := store.PostgresLocation("postgres://alice:s3cret@db.example.com:5433/central?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	s := postgresAt{store.NewMemory(), l}
	c := core.New(s, clock.NewFake(t0), &ids.Sequence{})
	v, err := c.GetStorage(ctx, api.GetStorageInput{})
	if err != nil {
		t.Fatal(err)
	}
	if v.Kind != "postgres" || v.Host != "db.example.com" || v.Port != 5433 || v.Database != "central" || v.Path != "" {
		t.Errorf("shows %+v, want postgres at db.example.com:5433/central", v)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "s3cret") || strings.Contains(string(b), "alice") {
		t.Errorf("the credential shows in %s", b)
	}
}
