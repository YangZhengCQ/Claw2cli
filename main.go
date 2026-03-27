package main

import "github.com/YangZhengCQ/Claw2cli/cmd"

var (
	// Set by goreleaser via -ldflags at build time.
	version = "dev"  //nolint:unused // set via ldflags
	commit  = "none" //nolint:unused // set via ldflags
)

func main() {
	cmd.Execute()
}
