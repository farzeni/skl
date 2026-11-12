# skl

Your Claude Code skills, on any machine. Keep them in one repo, load any mix of them anywhere, roll back any change, and improve your skills by learning from other people's.

Docs: https://farzeni.github.io/skl/ (coming soon; until then, read them in [`docs/`](docs/)).

## Install

```sh
go install github.com/farzeni/skl/cmd/skl@latest
```

skl needs git 2.38 or later. Then follow [Getting started](docs/guide/getting-started.md).

## What works in v0.1

- `skl init <repo>`, with the offer to import the skills already in `~/.claude/skills`
- `use`, `add`, `drop`, named skill sets and `save`, in the user scope
- `status` and `history`, both with `--json`; `rollback`
- edits to loaded skills saved back to the vault
- `import`
- `sync`, with conflicts resolved by hand
- the `claude` and `agents` targets; `help` and `--version`

Next: `learn` (v0.2 copies, v0.3 lessons), then `open`, project scope and shell completion (v0.4). The full table is in the [roadmap](docs/design/roadmap.md#what-works-in-v0-1). Pages that describe unbuilt parts say so at the top.

## Develop

- Code: `go run ./cmd/skl --help`, `make check`; see [CONTRIBUTING.md](CONTRIBUTING.md)
- Docs: `cd docs && pnpm install && pnpm dev`
- Demo recording: `cd demo && vhs demo.tape` (needs [vhs](https://github.com/charmbracelet/vhs); writes `docs/public/demo.gif` using the scripted mock in `demo/bin/skl`)
