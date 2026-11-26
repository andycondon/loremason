# loremason

A context graph for AI agent work. Small, typed, linked records in plain files that keep track of what we believe and why, what we chose to do, and what to reopen when a belief turns out to be wrong.

> **Status: early.** The design is written; the tool is being built up milestone by milestone. Nothing here is stable yet.

## Why

Agents doing real work need the right context at the right moment. Hand-kept notes go stale, mix everything together, and rely on someone remembering what matters. loremason replaces them with records that:

- **trace every claim** back to the evidence and source behind it;
- **know what a change touches**, so a fallen belief reopens only the work built on it;
- **stay quiet**, so most changes are handled by agents and a person sees only proposals worth their attention;
- **survive interruption**, so any agent can pick up where another left off, from the files alone.

The files are the source of truth. They're readable in an editor, reviewable in a pull request, and mergeable like code. A SQLite index built from them answers questions such as "what is stale?" and "what relies on this?".

## Design

| Part | What it covers |
|---|---|
| [1. The model](docs/context-graph/01-model.md) | The record types, how they link, and the principles behind them |
| [2. The landscape](docs/context-graph/02-landscape.md) | How others solve the same problems, and what we borrowed |
| [3. The implementation](docs/context-graph/03-implementation.md) | Files, schemas, pins and fingerprints, the index, and the `mason` command |

## Install

Needs [Go](https://go.dev/dl/) 1.21 or later; the toolchain named in `go.mod` (Go 1.27) is fetched automatically.

```sh
go install github.com/andycondon/loremason/cmd/mason@latest
mason --version
```

`go install` puts `mason` in `$(go env GOPATH)/bin` (usually `~/go/bin`), which needs to be on your `PATH`.

## Use

A workspace is any folder with a `.loremason/config.yaml` naming the loremason version it uses:

```sh
mkdir .loremason && echo "version: 0.0.1" > .loremason/config.yaml
mason new belief --title "validate doesn't handle non-UTF-8 input"   # prints graph/belief/bel_….md
mason validate                                                      # checks every record; exits 1 on errors
```

`mason` finds the workspace by walking up from the current folder, the way git finds `.git`. Set `LOREMASON_WORKSPACE` to point it at another. It stops at the first `.loremason/` it finds, so a marker without a `config.yaml` is an error rather than skipped.

Each record type's fields are defined by a JSON Schema in [`schema/`](schema/), and [`schema/types.yaml`](schema/types.yaml) holds what JSON Schema can't say: ID prefixes, which edges point where, and which fields make up a record's meaning.

## Development

```sh
go test ./...                # run the tests
go test ./cmd/mason -update  # rewrite the expected output of the validate fixtures
go run ./cmd/mason --version # run without installing
go install ./cmd/mason       # install from a clone
```

## Licence

[MIT](LICENSE)
