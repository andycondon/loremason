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

## Install (development)

Needs [uv](https://docs.astral.sh/uv/).

```sh
uv tool install --editable .
mason --version
```

Run the tests with `uv run pytest`.

## Licence

[MIT](LICENSE)
