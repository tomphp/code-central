package client

import "testing"

func TestATestCannotReachTheRealService(t *testing.T) {
	for _, bad := range []string{DefaultURL, "http://127.0.0.1:7733", "http://[::1]:7733/", "http://example.com:8080"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s was allowed", bad)
				}
			}()
			refuseRealService(bad)
		}()
	}
	for _, ok := range []string{"http://127.0.0.1:51234", "http://localhost:8123", "http://[::1]:9"} {
		refuseRealService(ok)
	}
}
