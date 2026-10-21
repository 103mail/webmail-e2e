package e2e

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// The godog options are plain test flags, so `go test ./... -godog.tags=...`
// works the way the README shows. Untagged runs skip what is still @wip.
var (
	tags   = flag.String("godog.tags", "~@wip", "tag expression selecting the stories to run")
	format = flag.String("godog.format", "progress", "godog output format")
	paths  = flag.String("godog.paths", "features", "comma-separated feature files or directories to run")
)

// TestFeatures runs every story under features/ against the compose stack in
// ../webmail-deploy, or the one WEBMAIL_E2E_DEPLOY points at, with its mail
// sink at WEBMAIL_E2E_MAILSINK. It runs on the stack's federation network
// (run.sh), so the default addresses are the names Docker gives the services.
func TestFeatures(t *testing.T) {
	deployDir := os.Getenv("WEBMAIL_E2E_DEPLOY")
	if deployDir == "" {
		deployDir = "../webmail-deploy"
	}
	// Absolute, because the steps hand it to `docker compose`, and the daemon
	// resolves the stack's bind mounts on the host (run.sh).
	deployDir, err := filepath.Abs(deployDir)
	if err != nil {
		t.Fatal(err)
	}
	sinkURL := os.Getenv("WEBMAIL_E2E_MAILSINK")
	if sinkURL == "" {
		sinkURL = "http://mailsink:4566"
	}
	stack, err := NewStack(deployDir, sinkURL)
	if err != nil {
		t.Fatal(err)
	}
	// Steps make time pass on the stack's clock instead of waiting for it.
	passTime = stack.passTime
	suite := godog.TestSuite{
		Name: "webmail-e2e",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			newWorld(stack).register(sc)
		},
		Options: &godog.Options{
			Format:   *format,
			Paths:    strings.Split(*paths, ","),
			Tags:     *tags,
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("stories failed")
	}
}
