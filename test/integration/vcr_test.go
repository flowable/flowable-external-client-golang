package integration_test

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/flowable/flowable-external-client-golang/flowable"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

func cassettesDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "cassettes")
}

// setupVCR creates a go-vcr recorder for the current test.
// On first run it records HTTP interactions to a YAML cassette file.
// On subsequent runs it replays from the cassette (no network needed).
// Set VCR_RECORD=all to force re-recording.
func setupVCR(t *testing.T) {
	t.Helper()

	dir := cassettesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("failed to create cassettes dir: %v", err)
	}

	cassettePath := filepath.Join(dir, t.Name())

	mode := recorder.ModeRecordOnce
	if os.Getenv("VCR_RECORD") == "all" {
		mode = recorder.ModeRecordOnly
	}

	rec, err := recorder.New(cassettePath,
		recorder.WithMode(mode),
		// Match only on Method + URL to avoid multipart boundary mismatches
		recorder.WithMatcher(func(r *http.Request, cr cassette.Request) bool {
			return r.Method == cr.Method && r.URL.String() == cr.URL
		}),
		recorder.WithReplayableInteractions(true),
	)
	if err != nil {
		t.Fatalf("failed to create VCR recorder: %v", err)
	}

	client := rec.GetDefaultClient()

	// Inject VCR transport into the flowable package and test helpers
	flowable.SetHTTPClient(client)
	testHTTPClient = client

	t.Cleanup(func() {
		rec.Stop()
		flowable.SetHTTPClient(&http.Client{})
		testHTTPClient = http.DefaultClient
	})
}
