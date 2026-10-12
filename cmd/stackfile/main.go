// Command stackfile reads a project's stackfile.yml: what it builds with, how
// to use it, what to avoid, and which decisions back each choice. It is
// written to be called by coding agents as often as by people.
package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

// go install applies no ldflags, so fall back to the module version the go
// command records in the binary. Local builds have none and stay "dev".
func init() {
	if version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = strings.TrimPrefix(info.Main.Version, "v")
	}
}

// Exit codes. A hook decides whether to block a command from these alone, so
// each outcome it might treat differently gets its own code.
const (
	exitOK = 0
	// exitGate: the question was answered and the answer is no. The file
	// has errors, the package is not allowed, the block is stale.
	exitGate = 1
	// exitError: stackfile itself could not answer. No file, unreadable
	// file, bad arguments.
	exitError = 2
	// exitUndeclared: can-i add found nothing either way. Kept apart from
	// exitGate so a hook can choose to warn rather than block.
	exitUndeclared = 3
)

// exitCode carries a non-zero exit through cobra once the output has been
// written, so main does not print anything further.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit %d", int(e)) }

func main() {
	err := newRootCmd().Execute()
	var code exitCode
	switch {
	case err == nil:
		os.Exit(exitOK)
	case errors.As(err, &code):
		os.Exit(int(code))
	default:
		fmt.Fprintln(os.Stderr, "stackfile:", err)
		os.Exit(exitError)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "stackfile",
		Short: "Tell people and coding agents what this project builds with",
		Long: `stackfile reads stackfile.yml: what a project builds with, how to use it,
what is still undecided, what not to add, and which decisions back each
choice.

It finds the nearest stackfile.yml in the current directory or a parent, the
way git finds its repository. Use --file to point at one directly.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringP("file", "f", "", "path to stackfile.yml (default: the nearest one)")
	root.AddCommand(newValidateCmd(), newExplainCmd(), newCanICmd(), newRenderCmd(), newSchemaCmd())
	return root
}
