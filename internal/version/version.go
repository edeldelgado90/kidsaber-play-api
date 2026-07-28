// Package version exposes the build identity of the running binary.
//
// Version is stamped at build time from the Git commit that produced the image:
//
//	go build -ldflags "-X github.com/kidsaber/kidsaber-play-api/internal/version.Version=$GIT_SHA"
//
// Both Dockerfiles pass it through the VERSION build argument, which the deploy
// workflow fills with the commit SHA. Without that flag the value stays "dev",
// which is what local builds and tests see.
package version

// Version is the Git commit SHA the binary was built from, or "dev".
var Version = "dev"
