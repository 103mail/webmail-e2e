package e2e

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// The godog options are plain test flags, so `go test ./... -godog.tags=...`
// works the way the README shows. Untagged runs skip what is still @wip.
var (
	tags   = flag.String("godog.tags", "~@wip", "tag expression selecting the stories to run")
	format = flag.String("godog.format", "pretty", "godog output format")
	paths  = flag.String("godog.paths", "features", "comma-separated feature files or directories to run")
)

// TestFeatures runs every story under features/ against the compose stack in
// ../webmail-server/deploy, or the one WEBMAIL_E2E_DEPLOY points at, with its
// mail sink at WEBMAIL_E2E_MAILSINK.
func TestFeatures(t *testing.T) {
	deployDir := os.Getenv("WEBMAIL_E2E_DEPLOY")
	if deployDir == "" {
		deployDir = "../webmail-server/deploy"
	}
	sinkURL := os.Getenv("WEBMAIL_E2E_MAILSINK")
	if sinkURL == "" {
		sinkURL = "http://127.0.0.1:8025"
	}
	stack, err := NewStack(deployDir, sinkURL)
	if err != nil {
		t.Fatal(err)
	}
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
