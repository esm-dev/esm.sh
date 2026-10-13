package server

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestMain probes the network before running the tests: OS-level firewalls
// (e.g. the Windows Firewall) show a per-binary allow dialog on the first
// real network request, and every dial fails until the user confirms it.
// A probe loop absorbs that wait so the tests do not fail on their first
// dial, and gives you the time to hit the allow button.
func TestMain(m *testing.M) {
	start := time.Now()
	fmt.Fprintln(os.Stderr, "probing network access… (if a firewall prompt shows up, allow it)")
	for {
		response, err := (&http.Client{Timeout: 5 * time.Second}).Get("https://registry.npmjs.org/proxy")
		if err == nil {
			response.Body.Close()
			fmt.Fprintf(os.Stderr, "network access confirmed in %dms\n", time.Since(start).Milliseconds())
			break
		}
		if time.Since(start) > 10*time.Second {
			fmt.Fprintf(os.Stderr, "network probe failed: %v\n", err)
			break
		}
		time.Sleep(time.Second)
	}
	os.Exit(m.Run())
}
