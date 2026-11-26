# CLAUDE.md

Guidance for agents working on loremason.

- **Read [docs/development.md](docs/development.md) before changing code.** It holds the principles, the test layout and the steps for building a milestone. This file doesn't repeat them.
- **The design is in [docs/context-graph/](docs/context-graph/).** When code and design disagree, say so; don't quietly follow either.
- **Model first.** Don't write code for a new concept until its types and methods are written down and a person has reviewed them.
- **Before committing:** `gofmt -l .`, `go vet ./...` and `go test -race ./...` must all be clean.
- **This repository is public.** Commits, comments and docs don't mention private notes, people's details or IDs from outside this repository.
