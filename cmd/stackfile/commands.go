package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/JustSteveKing/stackfile/internal/check"
	"github.com/JustSteveKing/stackfile/internal/deps"
	"github.com/JustSteveKing/stackfile/internal/render"
	"github.com/JustSteveKing/stackfile/schema"
	"github.com/JustSteveKing/stackfile/stack"
)

// locate returns the stackfile to use: --file when given, the nearest one
// otherwise.
func locate(cmd *cobra.Command) (string, error) {
	if p, _ := cmd.Flags().GetString("file"); p != "" {
		return p, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return stack.Find(wd)
}

// load returns a stackfile that is safe to act on. Warnings are fine;
// errors are not, because an agent following a broken file is worse off than
// one with no file at all.
func load(cmd *cobra.Command) (*stack.File, error) {
	path, err := locate(cmd)
	if err != nil {
		return nil, err
	}
	f, ds, err := stack.Load(path)
	if err != nil {
		return nil, err
	}
	if f == nil || ds.HasErrors() {
		return nil, fmt.Errorf("%s has %d error(s); run 'stackfile validate' to see them", displayPath(path), ds.Count(stack.SeverityError))
	}
	return f, nil
}

func formatFlag(cmd *cobra.Command) {
	cmd.Flags().String("format", "text", "output format: text or json")
}

func format(cmd *cobra.Command) (string, error) {
	f, _ := cmd.Flags().GetString("format")
	if f != "text" && f != "json" {
		return "", fmt.Errorf("unknown --format %q; use text or json", f)
	}
	return f, nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// displayPath shows a path relative to the working directory when it is
// under it, which is how editors and terminals make it clickable.
func displayPath(p string) string {
	wd, err := os.Getwd()
	if err != nil {
		return p
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if rel, err := filepath.Rel(wd, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [file]",
		Short: "Check a stackfile against the schema and for contradictions",
		Long: `validate checks stackfile.yml against the JSON Schema, then for what the
schema cannot see: duplicate names, replacements that are not declared,
fields left over from an earlier status, and versions written as numbers.

Errors exit 1. Warnings do not, unless --strict is given.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := format(cmd)
			if err != nil {
				return err
			}
			path := ""
			if len(args) == 1 {
				path = args[0]
			} else if path, err = locate(cmd); err != nil {
				return err
			}
			_, ds, err := stack.Load(path)
			if err != nil {
				return err
			}
			strict, _ := cmd.Flags().GetBool("strict")
			failed := ds.HasErrors() || (strict && len(ds) > 0)

			w := cmd.OutOrStdout()
			name := displayPath(path)
			if out == "json" {
				if ds == nil {
					ds = stack.Diagnostics{}
				}
				if err := writeJSON(w, map[string]any{"file": name, "valid": !failed, "diagnostics": ds}); err != nil {
					return err
				}
			} else {
				for _, d := range ds {
					fmt.Fprintf(w, "%s:%s\n", name, d)
				}
				errs, warns := ds.Count(stack.SeverityError), ds.Count(stack.SeverityWarning)
				if errs == 0 && warns == 0 {
					fmt.Fprintf(w, "%s: valid\n", name)
				} else {
					fmt.Fprintf(w, "%s: %d error(s), %d warning(s)\n", name, errs, warns)
				}
			}
			if failed {
				return exitCode(exitGate)
			}
			return nil
		},
	}
	cmd.Flags().Bool("strict", false, "treat warnings as errors")
	formatFlag(cmd)
	return cmd
}

func newExplainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain [path]",
		Short: "Show what applies at a path: tools, rules, commands, what not to add",
		Long: `explain prints what an agent needs before working at a path: the entries in
scope there with their rules and commands, what is still undecided, and what
must not be added. Without a path it covers the whole project.

The path may be a file or a directory, and need not exist yet. For a
directory, entries scoped to anything inside it are included.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := format(cmd)
			if err != nil {
				return err
			}
			f, err := load(cmd)
			if err != nil {
				return err
			}
			rel, isDir := ".", true
			if len(args) == 1 {
				if rel, isDir, err = relative(f.Dir, args[0]); err != nil {
					return err
				}
			}
			if out == "json" {
				return writeJSON(cmd.OutOrStdout(), render.ExplainJSON(f, rel, isDir))
			}
			render.Explain(cmd.OutOrStdout(), f, rel, isDir)
			return nil
		},
	}
	formatFlag(cmd)
	return cmd
}

// relative turns a path given on the command line into one relative to the
// stackfile, as scope globs are written.
func relative(base, p string) (string, bool, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false, err
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, fmt.Errorf("%s is outside the project %s describes (%s)", p, stack.FileName, base)
	}
	// A path that does not exist yet is a file someone is about to create.
	info, err := os.Stat(abs)
	isDir := err == nil && info.IsDir()
	return filepath.ToSlash(rel), isDir, nil
}

func newCanICmd() *cobra.Command {
	canI := &cobra.Command{
		Use:   "can-i",
		Short: "Ask whether something is allowed",
	}
	add := &cobra.Command{
		Use:   "add <package>",
		Short: "Ask whether a package may be added, and what to do if not",
		Long: `can-i add answers whether a package may be added to the project, and what to
do instead when it may not. The package may carry a version as typed on an
install command line: axios@1.7, laravel/framework:^12.0.

Exit codes, for hooks:
  0  allowed (adopted, or on trial within its scope)
  1  not allowed (avoid list, hold, retired, or undecided)
  2  stackfile could not answer
  3  undeclared: the stackfile says nothing either way`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := format(cmd)
			if err != nil {
				return err
			}
			f, err := load(cmd)
			if err != nil {
				return err
			}
			eco, _ := cmd.Flags().GetString("ecosystem")
			ans := f.Lookup(stack.PackageName(args[0]), eco)

			if out == "json" {
				if err := writeJSON(cmd.OutOrStdout(), ans); err != nil {
					return err
				}
			} else {
				render.Answer(cmd.OutOrStdout(), ans)
			}
			switch ans.Verdict {
			case stack.VerdictAllowed, stack.VerdictTrial:
				return nil
			case stack.VerdictUndeclared:
				return exitCode(exitUndeclared)
			default:
				return exitCode(exitGate)
			}
		},
	}
	add.Flags().StringP("ecosystem", "e", "", "only match entries in this ecosystem (composer, npm, go, ...)")
	formatFlag(add)
	canI.AddCommand(add)
	return canI
}

func newRenderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Write the stack summary into AGENTS.md or CLAUDE.md",
		Long: `render writes a short summary of the stack into an agent instructions file,
between marker comments, leaving the rest of the file alone. Without markers
the block is appended. The file is created if it does not exist.

Use --check in CI or a pre-commit hook: it changes nothing and exits 1 when
the block is out of date.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := load(cmd)
			if err != nil {
				return err
			}
			block := render.Agents(f)
			if stdout, _ := cmd.Flags().GetBool("stdout"); stdout {
				fmt.Fprint(cmd.OutOrStdout(), block)
				return nil
			}

			target, _ := cmd.Flags().GetString("target")
			if !filepath.IsAbs(target) {
				target = filepath.Join(f.Dir, target)
			}
			current, err := os.ReadFile(target)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			next, err := render.Splice(string(current), block)
			if err != nil {
				return fmt.Errorf("%s: %w", displayPath(target), err)
			}

			if check, _ := cmd.Flags().GetBool("check"); check {
				if next != string(current) {
					fmt.Fprintf(cmd.OutOrStdout(), "%s is out of date; run 'stackfile render'\n", displayPath(target))
					return exitCode(exitGate)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s is up to date\n", displayPath(target))
				return nil
			}
			if next == string(current) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is up to date\n", displayPath(target))
				return nil
			}
			if err := os.WriteFile(target, []byte(next), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", displayPath(target))
			return nil
		},
	}
	cmd.Flags().String("target", "AGENTS.md", "file to write, relative to the stackfile")
	cmd.Flags().Bool("check", false, "change nothing; exit 1 if the block is out of date")
	cmd.Flags().Bool("stdout", false, "print the block instead of writing it")
	return cmd
}

func newCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Compare the stackfile with what the project installs",
		Long: `check reads the manifests and lockfiles next to stackfile.yml and reports
where they and the stackfile disagree:

  error    a package on the avoid list is a direct dependency
  error    a candidate for an undecided entry is already a direct dependency
  error    an installed version does not satisfy the declared one
  warning  a retired entry is still a direct dependency
  warning  a package on the avoid list comes in through another package
  warning  an adopted entry is not installed
  note     a direct dependency the stackfile does not mention (--undeclared)

Read: composer.json and composer.lock; package.json with package-lock.json
or bun.lock; go.mod. An ecosystem with no manifest is skipped.

Errors exit 1. Warnings do not, unless --strict is given. Notes never do.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := format(cmd)
			if err != nil {
				return err
			}
			f, err := load(cmd)
			if err != nil {
				return err
			}
			invs, err := deps.Read(f.Dir)
			if err != nil {
				return err
			}
			undeclared, _ := cmd.Flags().GetBool("undeclared")
			res := check.Run(f, invs, check.Options{Undeclared: undeclared})

			if out == "json" {
				if err := writeJSON(cmd.OutOrStdout(), res); err != nil {
					return err
				}
			} else {
				render.Check(cmd.OutOrStdout(), displayPath(f.Path), res)
			}
			strict, _ := cmd.Flags().GetBool("strict")
			if res.Count(check.SeverityError) > 0 || (strict && res.Count(check.SeverityWarning) > 0) {
				return exitCode(exitGate)
			}
			return nil
		},
	}
	cmd.Flags().Bool("strict", false, "treat warnings as errors")
	cmd.Flags().Bool("undeclared", false, "list direct dependencies the stackfile does not mention")
	formatFlag(cmd)
	return cmd
}

func newSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema a stackfile is validated against",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cmd.OutOrStdout().Write(schema.JSON)
			return err
		},
	}
}
