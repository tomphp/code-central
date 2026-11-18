package app

import "time"

// ServeOn exposes the serve-and-shut-down loop to the shutdown test.
var ServeOn = serveOn

// RenewEvery makes the services a test starts renew their Registrations
// and Leases every d, until it ends; a test that calls it isn't parallel.
func RenewEvery(t interface{ Cleanup(func()) }, d time.Duration) {
	was := serviceRenewal
	serviceRenewal = d
	t.Cleanup(func() { serviceRenewal = was })
}
