// Package version exposes the build-time application version string.
package version

// Version is the build-time application version. It defaults to "dev" and is
// overridden at image build via
// -ldflags "-X github.com/skua-app/skua/internal/version.Version=<v>".
var Version = "dev"
