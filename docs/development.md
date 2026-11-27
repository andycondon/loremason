# Developing loremason

How the code is organised, and the principles it follows. Read this before adding a command or a check. The design of the graph itself is in [`context-graph/`](context-graph/); this page is about the code that implements it.

## The shape of the code

```
cmd/mason/          the commands: each calls the domain and prints what it returns
internal/cli/       commands, flags, help and exit codes; knows nothing about loremason
internal/graph/     the model: Graph, File, Node, Edge, Ref, Pin; Load and Create
internal/check/     rules over a loaded graph
internal/index/     a SQLite index of the graph, rebuilt when the files change, and the queries over it
internal/pinning/  sync and revise: plan the changes, then apply them
internal/page/     the generated initiative pages: render them, check them against disk, plan what sync writes
internal/fingerprint/ the fp1 method, pure functions
internal/splice/   rewrite the header fields tools own; leave every other byte
internal/record/    reading a record file (Parse; Map, Value and Path over the header)
                    and writing YAML in the one form tools use (Out, Render, Rewrite)
internal/registry/  the record types (schema/types.yaml) and their JSON Schemas
internal/diag/      errors and warnings, each at a place in a file
internal/workspace/ finding the workspace and reading its config
internal/id/        making and recognising IDs
internal/suggest/   "did you mean"
internal/testws/    workspaces for tests
schema/             types.yaml and one JSON Schema per record type, embedded
testdata/           workspaces the tests load
```

Start reading at `internal/graph/graph.go`, then `load.go`, then `internal/check/check.go`.

## Principles

**Model the domain first.** Before writing a command or a check, name the types it works on and what they can answer. A link is an `Edge` with a target `Ref`, not a string in a map. A record is a `Node` with a `registry.Kind`. New features start by extending the model, then use it.

**Untyped data stays at the edges.** YAML nodes are wrapped by `record.Map` and `record.Value` as soon as a file is parsed. The header as plain values (`Record.Data`) exists only because the JSON Schema library needs it. Nothing else should read it.

**Ask the model; don't reach through it.** When a caller needs something like "do these two records look the same?", it calls `node.Identity()` instead of assembling the answer from `node.File.Type.Identity` and the header. If a caller reaches more than two levels deep, or repeats a nil check or a string operation, the answer probably belongs on a type as a method.

**Each rule of syntax has one home.**
- References (`bel_7k3m9q/s1@r2`) are read and written only in `graph/ref.go`.
- The file layout (`graph/<folder>/<id>.md`) lives only in `graph/layout.go`.
- Field paths (`support[0].direction`) are written only by `record.Path`.
- A revision entry is read and written only in `graph/revision.go`, and a pinned link (`{to, rev, fp, via}`) only in `graph/pin.go`. Tools decide what they say; those files decide how it's written.
- Whether a file is safe to write is decided once, by `check` (`Result.Broken`), and every writing tool takes that set.

When a rule is also stated in a schema, a test keeps the two in agreement, as `TestParseRefAgreesWithTheSchema` does.

**Loading and checking have a fixed boundary.** `graph.Load` reports only what keeps a file, name or link out of the graph: a folder no type has, a second record with the same ID, an alias already taken. Everything that made it into the graph but is wrong (a link to the wrong type, a field the schema rejects) is found by a rule in `check`. When the schema can report a bad value, the loader skips that value silently rather than reporting it twice.

**The command stays thin.** A command is a `cli.Command`: it declares its arguments and flags, calls the domain, prints the result, and returns an error. Package `cli` does the parsing, the help and the exit codes, and imports nothing from loremason, so it stays general. Domain rules, such as what counts as a valid title or which type a name means, live in the domain packages, so a later caller (a skill, a server) gets the same rules.

**Tools that write plan first.** A writing command decides everything it will change, as a list of actions said in plain words, before it touches a file. Then it applies them. That gives `--dry-run` for free, and lets tests check the plan without writing files. Writes go through `splice`, so only the fields a tool owns are rewritten, and each file is replaced in one step.

**Messages are for people.** Every error names the file, the line and the field, says what was wrong in plain words, and suggests a fix when one is likely. Wording helpers live in `check/words.go`.

## Tests

- **Test a rule in its own package.** Check rules are tested in `check`, pinning in `pinning`, parsing and help in `cli`. The command's tests in `cmd/mason` cover only the wiring: that each command calls the domain, prints what it returns and exits with the right code, with one test per path. If a command test is the only thing proving a rule, the test is in the wrong place.
- **Fixture workspaces.** Each folder in `testdata/check/` is a small workspace for one check rule, with `want.txt` holding exactly the diagnostics it reports. To add a rule, add a fixture. To accept a wording change, run `go test ./internal/check -update` and review the diff.
- **The worked example.** `testdata/worked-example/` is the design's worked example as records, and it must always check clean. The design's "a belief changes" walk-through runs as tests in `pinning`.
- **Test files mirror source files.** `load.go` is tested in `load_test.go`, in the same order as the code, with sections marked `// ---- … ----`. Shared helpers go in `helpers_test.go`. A test writes the records it needs inline, so what it proves is on the page; fixture folders are for `check`, whose rules need whole workspaces.
- **Workspaces in tests.** `internal/testws` writes records into a temporary workspace and reads, edits and loads them, so tests in every package set up the same way.
- **Tests that can't pass by accident.** A test that loads a workspace fails if the workspace has no records, so a wrong path can't pass as an empty graph.

Run `go vet ./... && go test -race ./...` before committing. CI runs the same, plus `gofmt`.

## Doc comments

A comment is checked like code: it must be true of the code as it is now.

- **Say what it's for, or how it differs from its neighbours.** `Node`, `Lookup` and `Resolve` all return a node; their comments say which takes an ID, which parses text and returns an error, and which takes a parsed reference and returns nil.
- **Describe today's code.** Plans belong in the task, not in a comment. Re-read the comments of anything that moved or changed meaning, after every restructure.
- **Name what callers need** when it isn't obvious: what a flag means, which exit code, when it panics.
- **One home per rule.** Explain a rule where it's implemented; elsewhere, point there.
- **Fields that mirror a config file** each get one line, worded like the config's key.
- **Go's form:** start with the name and a verb ("Lookup returns…", "Sealed reports whether…"). Package comments state the design and point to the section of the design doc they implement.
- **No padding.** Small unexported helpers whose name says it all, and `String` and `Error` methods, can go without.

## Building a milestone

Each milestone, or each task within one, goes through the same steps:

1. **Code model first.** The task says what should happen; it doesn't name types or packages. Before any code, the builder writes down the new types, the package each lives in, the methods callers will use, and which package's tests will prove each acceptance criterion. A person reviews it before any code is written. Criteria describe behavior, so code with no model at all can meet them; this step is what prevents that.
2. **Build** against the model. Report any departure from it, and why.
3. **Review** by someone who didn't write it, on four concerns: correctness, design (the principles above), tests (each rule proven in its own package) and doc comments.
4. **Comment pass.** After the review's fixes, re-read every comment on code that changed.
