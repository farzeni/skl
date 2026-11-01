# Contributing to skl

The docs in `docs/` are the spec. A behavior is written there first, from the user's side, and the code follows. If the code shows a doc is wrong or silent, the docs change first, in their own PR.

## Setup

You need Go 1.26 (the latest patch), git 2.38 or later, and for the docs pnpm.

```sh
go run ./cmd/skl --help   # run skl from the checkout
make check                # everything CI runs
```

| Target | Does |
|---|---|
| `make check` | Format check, lint, `go test -race ./...`, govulncheck and `goreleaser check`. CI runs exactly this, on Linux and macOS. |
| `make test` | `go test -race ./...` |
| `make fmt` | Format the code (gofumpt, goimports). |
| `make golden` | Rewrite the expected output: `UPDATE=1 go test ./...`. Review the diff before you commit it. |
| `make docs` | Build the docs site. A dead link fails the build. |
| `make build` | Build `bin/skl`, with the commit of this checkout. A plain `go build` in a git worktree under `.claude/worktrees/` stamps the main checkout's commit, because Go only takes a `.git` folder for a repo's root and a worktree's `.git` is a file. |

The dev tools are pinned in `tools/go.mod`, apart from the main `go.mod`, so their dependencies never change skl's. `make` runs them with `go tool -modfile=tools/go.mod <tool>`. GoReleaser comes from your `PATH`, else `go run` at the version the Makefile pins.

## Layout

```
cmd/skl/          main: cli.Run, then exit
internal/cli/     parse the line, command table, help, error → ✗ line and exit code
internal/errs/    errs.Hinted (✗ msg · hint, exit 1), errs.Usage (exit 2)
internal/app/     one file per command, and the frame every change runs in
internal/…        domain packages: see docs/design/architecture.md "Implementation"
spec/             the docs' statements as testscript scripts
tools/            go.mod pinning golangci-lint, gofumpt, govulncheck
```

## Conventions

**Dependencies.** Standard library first. Each new module is justified in the PR that adds it.

**Context.** Every function that does I/O takes `ctx context.Context` first. Ctrl-C cancels it; a command returns the `context.Canceled` error, and skl exits 130. A step that must not stop halfway, such as swapping loaded folders or a commit, runs under `context.WithoutCancel(ctx)` and is kept short. A second Ctrl-C kills the process.

**Errors.**
- Domain packages return wrapped errors, with sentinels or types the caller checks with `errors.Is` and `errors.As` (`vault.ErrNoVault`, `gitx.ErrUnreachable`).
- Only `app` adds the command the user should run: `errs.Hint(err, "skl init <repo>")`. A mistake in the command line is `errs.Usagef(hint, …)`.
- Only `cli` prints `✗` and picks the exit code: no error 0, `errs.Usage` 2, `context.Canceled` 130, anything else 1.
- A warning is never an error. It is a `!` line, and the exit code stays 0.

**Output.** Only `ui` writes to the terminal. What the user asked for (a report, `--json`, a `--dry-run` plan, help) goes to stdout; marks and questions go to stderr. The width is the terminal's, else `COLUMNS`, else 80; a hint that doesn't fit moves to the next line after `·`. Every message uses the vocabulary in `docs/reference/commands.md` "Output".

**Logging.** `log/slog` on stderr, only with `SKL_DEBUG=1`. `gitx` logs each git command before it runs. There is no other logging.

**State.** No global mutable state and no `init()`, except the `ext` registry and `thirdparty/` parts that register with it. Read-only tables are fine. Dependencies such as the streams, config and clock are passed in.

**Interfaces.** Declared by the package that uses them, small (one to three methods), and only where a test needs a fake. Git is not faked: tests run real git on temporary repos. To test a failure such as an unreachable remote, wrap the runner.

**Commands appear when built.** The command table in `internal/cli` holds only commands and flags that work. Help never lists anything else.

**Flags.** skl parses its own arguments; never use the `flag` package. Flags are `--long` only, as `--x v` or `--x=v`, and can go anywhere. `-h` is the one short flag. A flag name means the same thing, with or without a value, in every command.

## Tests

Three layers, each as fast and offline as the code allows:

1. **Table tests** for pure logic: the parser, selectors, resolution, refs, config precedence, rendering. Rendered output is compared with `testdata/*.golden`; a test that compares with a file rewrites it when `UPDATE=1` is set.
2. **Package tests** with `t.TempDir()`, `t.Setenv("HOME", …)` and real git, for `gitx`, `vault`, `load` and `state`.
3. **Spec tests** in `spec/`: [testscript](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) scripts that run `skl` as its own process, through the real `main` path.

### Spec scripts

One script per doc page or section: `spec/testdata/<page>[-<section>].txtar`, such as `skill-sets.txtar` for `docs/guide/skill-sets.md` or `skill-sets-status.txtar` for one of its sections. The first line names the page; each block starts with the doc sentence it checks, quoted:

```
# docs/reference/commands.md

# "A mistyped command gets a suggestion and exits with code 2"
exitcode 2 skl hlep
cmp stderr unknown-hlep.txt

-- unknown-hlep.txt --
✗ unknown command hlep · skl help
```

- Each script runs in its own `$WORK` with `HOME=$WORK/home`, `COLUMNS=80`, and a git config that sends `https://github.com/<owner>/<repo>` to a bare repo under `$REMOTES/github.com/`. skl uses its real URLs; there is no test hook in skl.
- Helpers, defined in `spec/spec_test.go` (add one when a script needs it):
  - `remote <owner/repo> [dir]` creates that bare repo, with one commit holding `dir`'s files if given.
  - `exitcode <n> <program> [args…]` runs like `exec` and checks the exact exit code.
- `exec` is always explicit: `exec skl …`, `exec git …`.
- Prefer `cmp` against a file in the archive to a regexp; `UPDATE=1 go test ./spec` rewrites those files. Read the diff: it is the user-visible change.
- A card writes the scripts for the sentences it implements. A sentence left for later gets a script starting with `skip 'pending: <page>'`.
- `spec/testdata/harness/` tests the harness itself.

## Add a command

1. Its behavior is in the docs: `docs/reference/commands.md` and the guide page that explains it.
2. A spec script for those sentences, failing.
3. The command in `internal/app/<command>.go`.
4. Its entry in the command table in `internal/cli`, with its arguments, its flags, a one-line summary, its help and an example.
5. `make check`.

## Add a third-party part

A part that depends on someone else's service, tool or file format lives in `thirdparty/<name>/` and plugs into `internal/ext`. Core never assumes it exists. See `docs/design/architecture.md` "Third-party parts"; the mechanics come with the `ext` package.

## Workflow

Work is tracked on the [SKL Board](https://github.com/users/farzeni/projects/1): one card, one branch, one PR that says `Closes #<card>`. Never commit to `main`. Merge `main` into your branch when you need it; don't rebase or force-push a pushed branch.
