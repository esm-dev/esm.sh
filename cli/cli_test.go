package cli

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestCommandExitStatus(t *testing.T) {
	if os.Getenv("ESM_CLI_COMMAND_TEST") == "1" {
		os.Args = append(os.Args[:1], os.Args[slices.Index(os.Args, "--")+1:]...)
		flag.CommandLine = flag.NewFlagSet("esm.sh", flag.ExitOnError)
		Run()
		os.Exit(0)
	}
	for _, tc := range []struct {
		args []string
		html string
		err  string
	}{
		{args: []string{"add", "--no-prompt", "invalid package"}, err: "Failed to add imports"},
		{args: []string{"tidy"}, err: "index.html not found"},
		{args: []string{"tidy"}, html: `<script type="importmap">invalid</script>`, err: "invalid importmap script"},
		{args: []string{"add", "--help"}},
		{args: []string{"tidy", "--help"}},
	} {
		t.Run(strings.Join(tc.args, " ")+tc.html, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if tc.html != "" {
				if err := os.WriteFile("index.html", []byte(tc.html), 0644); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestCommandExitStatus$", "--"}, tc.args...)...)
			cmd.Env = append(os.Environ(), "ESM_CLI_COMMAND_TEST=1")
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()
			if tc.err == "" {
				if err != nil {
					t.Fatalf("help failed: %v: %s", err, &stderr)
				}
			} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 || !strings.Contains(stderr.String(), tc.err) {
				t.Fatalf("expected exit 1 and %q on stderr, got %v: %s", tc.err, err, &stderr)
			}
		})
	}
}
