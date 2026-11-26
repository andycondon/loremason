# Developing loremason

How the code is organised, and the principles it follows. Read this before adding a command or a check. The design of the graph itself is in [`context-graph/`](context-graph/); this page is about the code that implements it.

## The shape of the code

```
cmd/mason/          the command: parse arguments, call one domain function, print
internal/graph/     the model: Graph, File, Node, Edge, Ref, Pin; Load and Create
internal/check/     rules over a loaded graph
internal/record/    reading a record file: Parse, and Map, Value and Path over the header
internal/registry/  the record types (schema/types.yaml) and their JSON Schemas
internal/diag/      errors and warnings, each at a place in a file
internal/workspace/ finding the workspace and reading its config
internal/id/        making and recognising IDs
internal/suggest/   "did you mean"
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

When a rule is also stated in a schema, a test keeps the two in agreement, as `TestParseRefAgreesWithTheSchema` does.

**Loading and checking have a fixed boundary.** `graph.Load` reports only what keeps a file, name or link out of the graph: a folder no type has, a second record with the same ID, an alias already taken. Everything that made it into the graph but is wrong (a link to the wrong type, a field the schema rejects) is found by a rule in `check`. When the schema can report a bad value, the loader skips that value silently rather than reporting it twice.

**The command stays thin.** A command parses its arguments, calls one domain function, prints the result and picks the exit code. Domain rules, such as what counts as a valid title or which type a name means, live in the domain packages, so a later caller (a skill, a server) gets the same rules.

**Messages are for people.** Every error names the file, the line and the field, says what was wrong in plain words, and suggests a fix when one is likely. Wording helpers live in `check/words.go`.

## Tests

- **Fixture workspaces.** Each folder in `testdata/validate/` is a small workspace for one rule, with `want.txt` holding the exact output and exit code. To add a rule, add a fixture. To accept a wording change, run `go test ./cmd/mason -update` and review the diff.
- **The worked example.** `testdata/worked-example/` is the design's worked example as records, and it must always validate clean.
- **Tests that can't pass by accident.** A test that loads a workspace fails if the workspace has no records, so a wrong path can't pass as an empty graph.
- **Domain functions have their own tests** (`graph`, `registry`, `record`), not only the command's.

Run `go vet ./... && go test -race ./...` before committing. CI runs the same, plus `gofmt`.

## Before building a milestone

Write down its code model first: the new types, which package they live in, and the methods callers will use. Get that reviewed, then write the code. Acceptance criteria that only describe output can be met by code with no model at all.
