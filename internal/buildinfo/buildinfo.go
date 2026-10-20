// Package buildinfo holds the version string stamped into the binaries at
// build time. cmd/sendry and cmd/sendry-web copy their ldflags values here so
// that library code (for example the /health handler) reports the real
// version instead of a constant.
package buildinfo

// Version is the release version, e.g. "0.4.21". "dev" when built without ldflags.
var Version = "dev"
