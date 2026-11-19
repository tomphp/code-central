package api

// FactoryWake is what next_factory_wake returns: Woken when a change the
// Factory's poll acts on ended the call, and empty when it timed out, a
// newer call replaced it or its request ended.
type FactoryWake struct {
	Woken bool `json:"woken,omitempty"`
}

// NextFactoryWakeInput is the Factory's next_factory_wake(factory, host),
// named as its Heartbeat names it.
type NextFactoryWakeInput struct {
	Factory string `json:"-" query:"factory"`
	Host    string `json:"-" query:"host"`
}
