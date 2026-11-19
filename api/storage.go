package api

import (
	"time"
)

// GetStorageInput is a Storage read: one Project's counts, or every
// Project's (Archived ones too) when Project is "". The Store, the memory
// and the services are whole either way.
type GetStorageInput struct {
	Project string `json:"project,omitempty" query:"project" desc:"Count only this Project's records; every Project's when left out."`
}

// StorageView is get_storage.
type StorageView struct {
	// Kind is the Store Kind: files, sqlite or postgres; memory for the
	// in-memory fake.
	Kind string `json:"kind"`
	// Path is the File Store's folder or the SQLite database file; empty
	// for Postgres and the fake.
	Path string `json:"path,omitempty"`
	// Host, Port and Database are the Postgres Store's address, as its
	// URL names them, without its user or password; empty for the other
	// Kinds.
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
	// Size is the Store's size in bytes, Usage included, as its Kind can
	// tell it.
	Size int64 `json:"size"`
	// Projects are the Projects' counts, in key order: the scoped one's
	// alone, or every one's.
	Projects []StorageProject `json:"projects"`
	Memory   ProcessMemory    `json:"memory"`
	// Services are the live Service Registrations, in host order.
	Services []StorageRegistration `json:"services"`
}

// StorageProject is one Project's count of records.
type StorageProject struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Specs and Tickets are its Specs and Tickets, in every State.
	Specs   int `json:"specs"`
	Tickets int `json:"tickets"`
	// UsageDays is how many days of Usage it keeps.
	UsageDays int `json:"usage_days"`
}

// ProcessMemory is this service's memory.
type ProcessMemory struct {
	// Resident is the process's resident size in bytes, as the operating
	// system counts it; 0 where it doesn't say.
	Resident int64 `json:"resident,omitempty"`
	// Heap is the bytes of Go heap the process's objects occupy.
	Heap int64 `json:"heap"`
}

// StorageRegistration is a live Service Registration.
type StorageRegistration struct {
	Host    string    `json:"host"`
	Version string    `json:"version"`
	Renewed time.Time `json:"renewed"`
}
