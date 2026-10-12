# stackfile

A file format and CLI that tell people and coding agents what a project
builds with, how to use it, what is still undecided, and what not to add.

ADRs and RFCs record decisions one at a time. Nothing records the result: what
is true *now*. An agent dropped into a repository won't read forty ADRs to
find out that you use Postgres, that Redis was retired last spring, and that
the queue backend is still being argued about. It reaches for whatever is most
popular instead. `stackfile.yml` is the current answer, and each entry can
point at the decision behind it.

It is written first and ratified later. You can declare a stack on day one,
before any ADR exists, and it guides the specs, RFCs and ADRs that follow.
Each one that lands either ratifies an entry or amends the file.

## A stackfile

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/JustSteveKing/stackfile/main/schema/stackfile.schema.json
stackfile: 1
project: legacy-crm

principles:
  - Prefer boring, well-supported tech over novelty.

layers:
  backend:
    - name: laravel
      version: "^12.0"
      ecosystem: composer
      package: laravel/framework
      status: adopt
      basis: ratified
      decided_by: [ADR-0003]
      scope: ["app/**", "routes/**"]
      rules:
        - Controllers are invokable, one action per class.
      commands:
        test: php artisan test

  data:
    - name: postgres
      version: "17"
      status: adopt
      why: Relational data; JSONB covers the document cases.
    - name: redis
      ecosystem: composer
      package: predis/predis
      status: retired
      replaced_by: database-queue
      why: Queues and cache moved to Postgres.
    - name: search
      status: proposed
      candidates: [meilisearch, typesense]
      open: Do we need typo tolerance?

avoid:
  - name: axios
    ecosystem: npm
    package: axios
    use_instead: native fetch
    why: One HTTP client is enough.
```

The schema comment gives you completion and validation in any editor that
runs the YAML language server.

### Statuses

| Status | Meaning |
| --- | --- |
| `adopt` | The default choice. |
| `trial` | Allowed within its `scope` while it is evaluated. |
| `proposed` | Not decided. `candidates` lists the options, `open` the question. Agents are told to ask, not choose. |
| `hold` | No new uses. Existing ones may stay. |
| `retired` | Being removed. `replaced_by` names the entry to use instead. |

`basis` is `declared` (the default: stated, no decision record yet) or
`ratified` (backed by the ADRs or RFCs in `decided_by`). Both are binding.
The difference shows which choices have been argued and which were made on
instinct.

A stackfile lists what carries a decision, not every dependency. The lockfile
already does that.

## Commands

```bash
stackfile validate            # schema, then contradictions; errors exit 1, --strict for warnings too
stackfile explain [path]      # what applies at a path: tools, rules, commands, what not to add
stackfile can-i add <pkg>     # may this be added, and if not, what instead
stackfile render              # write the stack summary into AGENTS.md between markers
stackfile render --check      # for CI: exit 1 if that block is out of date
stackfile schema              # print the JSON Schema
```

Every command finds the nearest `stackfile.yml` from the current directory
up, like git. `-f` points at one directly. `validate`, `explain` and
`can-i add` take `--format json`.

`can-i add` exits 0 when allowed, 1 when not, 2 when it could not answer, and
3 when the stackfile says nothing either way, so a hook can block on 1 and
warn on 3.

## Getting agents to use it

1. **`stackfile render`** puts a short block in `AGENTS.md` (or `--target
   CLAUDE.md`): what to use, what not to add, what is undecided, and two
   instructions: run `stackfile explain` before working in a directory, and
   `stackfile can-i add` before adding a dependency.
2. **`explain` and `can-i add`** are written to be read by agents. A refusal
   names the replacement, the reason and the decision, and tells the agent to
   stop and ask rather than install the nearest alternative.
3. **CI** runs `validate` and `render --check` as the backstop.

## Install

```bash
go install github.com/JustSteveKing/stackfile/cmd/stackfile@latest
```

## Not yet

- `stackfile check`: compare the stackfile with `composer.lock`,
  `package-lock.json` and `go.mod`, catching anything on the avoid list that
  is installed and anything undeclared.
- A Claude Code hooks pack: `can-i add` before install commands, `explain` at
  session start.
- An MCP server, for agents other than Claude Code.
- `stackfile init`, drafting entries from the lockfiles.
- ADR and RFC frontmatter (`ratifies`, `amends`, `challenges`) and checking
  decision records against the stackfile, with archdoc.

## License

MIT
