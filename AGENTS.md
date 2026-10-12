# AGENTS.md

Guidance for coding agents working in this repository.

`README.md` is for people writing a stackfile and running the CLI.
`schema/stackfile.schema.json` is the format's specification. This file is
for anyone changing the code. The block at the end is generated from our own
`stackfile.yml`; edit that file and run `make render`, never the block.

## Commands

```bash
make check                                      # gofmt + tidy + vet + race tests + dogfood; what CI runs
go test ./stack/ -run TestParseRejects          # one group of cases
go run ./cmd/stackfile validate testdata/broken.yml
go run ./cmd/stackfile -f testdata/full.yml explain resources/views/admin
go run ./cmd/stackfile -f testdata/full.yml can-i add axios; echo $?
```

Go 1.27. Dependencies are cobra, `go.yaml.in/yaml/v3` and
`santhosh-tekuri/jsonschema/v6`, and that is deliberate.

## Architecture

```
schema/          the JSON Schema, embedded, and compiled once. The specification
stack/           load, validate (YAML, schema, semantics), and the queries: ForPath, Lookup
internal/glob    scope matching: path.Match per segment, plus **
internal/render  everything an agent or person reads: explain, can-i answers, the AGENTS.md block
cmd/stackfile    cobra: validate, explain, can-i add, render, schema
```

`schema` and `stack` are public so that other Go tools, archdoc first, can
read a stackfile without shelling out.

## The load-bearing decisions

**The schema is the specification.** Anything the format allows or forbids is
said in `stackfile.schema.json` first. Go code checks only what a schema
cannot express: uniqueness across the file, references between entries, and
fields that contradict each other. If you find yourself adding a structural
check in Go, put it in the schema.

**Three passes, and the second always runs when YAML parsed** (`Parse`).
YAML safety, then the schema, then semantics. One `validate` should report as
much as it can, so anchors and schema errors appear together. Semantics need
the decoded types, so they wait for the schema to pass.

**Every diagnostic has a line.** Schema errors come back as JSON pointers;
`nodeAt` walks them into the `yaml.Node` tree. An unknown key is reported at
the key, not at the object. A missing property is reported at the object that
lacks it.

**Versions are strings, and a number is caught before the schema sees it.**
`version: 1.10` is the float 1.1. The YAML pass reports it with the value
YAML turned it into and the quoted fix, and the schema's duplicate "got
number, want string" at the same path is dropped.

**No anchors, aliases or merge keys.** Agents edit this file. A value that
lives somewhere else is one an agent will change without seeing the other
uses.

**Messages are prompts.** Most output is read by an agent deciding what to do
next. Every refusal says what to use instead and ends by telling the agent to
stop and ask rather than install an alternative (`tellUser`). Undecided
entries say plainly that choosing is the user's call. Reword raw validator
messages in `schemaMessage` when they leave the reader to work out the fix.

**Exit codes are the hook contract** (`main.go`). 0 allowed, 1 answered no,
2 could not answer, 3 undeclared. A hook decides to block from these alone,
and undeclared is separate so a hook can warn on it instead. Changing them
breaks every hook built on them; `TestCanIAddExitCodes` holds them.

**Commands refuse a stackfile with errors** (`load`). An agent following a
broken file is worse off than one with none. Warnings do not stop anything.

**Undeclared is not forbidden.** A stackfile lists what carries a decision,
not every dependency; the lockfile already does that. So `can-i add` answers
"undeclared" for anything else, and says so without blocking.

**The rendered block is stable.** No dates, no counts that change without the
file changing. `render --check` compares bytes, and it is only useful if
rendering twice gives the same file.

## Fixtures

- `testdata/full.yml`: valid, and uses every status. Tests assert line
  numbers in it, so append rather than insert.
- `testdata/broken.yml`: fails the YAML and schema passes.
- `testdata/semantic.yml`: passes the schema, fails semantics.
- `testdata/basis.yml`: valid; a pending basis, `version_decided_by`, and
  proposals with one candidate and with none.
- `stackfile.yml` at the root is ours, and must stay clean under `--strict`.

<!-- stackfile:start: generated from stackfile.yml by 'stackfile render'; edit that file, not this block -->
## Stack

`stackfile.yml` is the source of truth for what this project builds with. If it and anything else disagree, it wins.

- Before working in a directory, run `stackfile explain <path>` for the rules that apply there.
- Before adding any dependency, run `stackfile can-i add <package>` and follow what it says.

**Principles**, the tie-breakers for anything not covered:

- One static binary; nothing to install before it runs, because hooks call it on every command.
- Prefer the standard library. A dependency has to save more than it costs to audit.
- The JSON Schema is the specification. Go types follow it, never the other way round.
- Every message an agent reads says what to do next, not only what is wrong.

**Use**

- go 1.27 (language)
- cobra ^1.10 (cli) in cmd/**
- yaml-v3 ^3.0 (format)
- jsonschema ^6.0 (format) in schema/**, stack/**

**Undecided.** Do not choose; ask the user:

- release-tooling: goreleaser or go install only. Ship binaries from v0.1 as legible does, or rely on go install until the format has settled?

**Do not add**

- viper: use flags read in each command instead. The only configuration is stackfile.yml itself, which has its own loader.
- doublestar: use internal/glob instead. Scopes need path.Match plus **, which is forty lines.

**Commands**

- check: `make check`
- build: `make build`
- dogfood: `go run ./cmd/stackfile validate --strict`
<!-- stackfile:end -->
