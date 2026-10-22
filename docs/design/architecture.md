# Architecture

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

## Principles

- **Git is the backend.** Your vault is a git repo, and other people's skills are read straight from theirs. Any git host works, and there's no server.
- **Copies, saved back.** skl loads a skill by copying it into the target folder, and records where each copy came from. Changes made to a copy are saved back to the vault, so any tool can edit a loaded skill.
- **The vault is the only thing you edit.** Other repos are read-only. Copies and lessons are written into your vault.
- **Nothing is lost.** Every change is a generation, and every content change is a commit.

## Loading pipeline

1. **Resolve.** Turn selectors into a flat list of skills, in order.
2. **Fetch.** Sparse-fetch any remote skill folder that isn't cached yet.
3. **Plan.** Diff against what's loaded now. `--dry-run` stops here.
4. **Apply.** Write the new copies in a staging folder, swap them in, then save a generation.

If a step fails, the previous set stays in place. A skill whose name is taken by a skill skl didn't create is dropped at the plan step, with a warning.

## Saving back

Each copy holds `.skl-copy.toml`: its source, the commit it was copied from, and a hash of its files. At the start of every command that changes something, skl hashes each copy and acts on the ones that changed:

- **A skill from your vault.** skl merges the copy into the vault with `git merge-file`, using the recorded commit as the base, and commits the result as a generation of its own. If both sides changed the same lines, it uses the sync conflict flow.
- **A skill from another repo.** skl stops and asks: save it to your vault as a copy with the changes, discard them, or keep the copy as it is. The cache is never written.

Only files inside a copy's folder are read. Anything next to it, such as skill-creator's `<skill>-workspace/`, is ignored.

## Sync

1. **Save back** changed copies, then **commit** hand edits in `~/.skl/vault`.
2. **Merge** the remote with `git fetch` and `git merge`. Never rebase.
3. **Push.**
4. **Reload** by running the loading pipeline on the selectors already loaded.

Loaded copies change only after the merge lands, so a conflict never reaches Claude Code. On a conflict, step 2 waits for the user: the learn engine resolves it, the user edits the markers and runs `skl sync --continue`, or `git merge --abort` undoes it.

## Fetching from remotes

skl runs git and behaves the same on every host.

- A skill folder is fetched with `git clone --depth 1 --filter=blob:none --sparse`, then `git sparse-checkout set <path>`. On a large repo that's a few hundred KB instead of the full history. Servers without filter support send everything: slower, but still correct.
- Local repos are cloned through `file://`, because git ignores filters for plain paths.
- Merge bases for `skl upstream` are fetched by commit, only when needed. `skl upstream --merge` rebuilds three versions of each file (the commit copied, the copy, the author's latest) and merges them with `git merge-file`.
- The cache lives at `~/.skl/cache/<host>/<path>`.
- Git runs with `GIT_TERMINAL_PROMPT=0` and `GIT_SSH_COMMAND="ssh -o BatchMode=yes"`, so a missing credential or an unknown host key fails at once instead of waiting for input.

## Learn engine

skl handles files, git and generations, and copies a skill you don't have without Claude. For a lesson, Claude Code does the analysis and the interview. skl prepares a workspace with `mine/`, `peer/` and a brief holding your text, then runs `claude` there with a bundled lesson skill. For a new skill built from text, `mine/` is empty. When you learn again from the same peer or a copy's source, it also adds `base/`, the version you copied or last learned from. A sync conflict uses the same engine with a bundled merge skill, and `base/`, `mine/` and `theirs/` for each conflicted file. Claude asks its questions in its own interview UI and writes the result to `result/`. skl then shows the diff and lands it.

## Guarantees

- Skills skl didn't create are never changed or removed. If one holds a name skl needs, skl skips its own skill.
- Every change can be rolled back, lessons included.
- A change made to a loaded copy is never discarded without asking.
- A remote skill is shown before its first use, and pinned to a commit and a content hash.
- Without skl, the vault is still an ordinary git repo of skills.

## Implementation

A single Go binary, so it installs anywhere with no runtime to set up. It calls the git CLI, which already provides sparse checkout, partial clone and three-way merge.

```
skl/
├── cmd/skl/        # commands
└── internal/
    ├── selector/   # parse selectors         (pure)
    ├── resolve/    # selectors → skill list  (pure)
    ├── vault/      # groups, skill sets
    ├── source/     # remote repos, cache
    ├── web/        # ref → web page URL     (pure)
    ├── load/       # plan, copy, save back, generations
    ├── upstream/   # source record, upstream merge
    ├── learn/      # workspace, claude engine, land
    └── ext/        # hooks for third-party parts
thirdparty/
├── skillssh/       # skl search, skills.sh links
└── npxskills/      # import source from the npx skills lock
```

## Third-party parts

Code that depends on someone else's service or file format lives in `thirdparty/<name>/`, never in `internal/`. Core offers three hooks in `internal/ext` and knows nothing about what plugs into them:

```go
// a new subcommand
func RegisterCommand(c Command)
// runs before a repo reference is parsed
func RegisterRefRewriter(f func(ref string) (string, bool))
// where a skill being imported came from
func RegisterImportSource(f func(name string) (ImportSource, bool))
```

Each part registers itself in `init()`, and `cmd/skl/thirdparty.go` imports it with one line. Deleting the package and that line removes the part: everything else builds and passes its tests. Its settings are a table in `config.toml` that only the part reads; core ignores tables it doesn't own. A part's failure stays inside its own command: the skills.sh part is the only code in skl that makes network requests other than git.

`selector` and `resolve` never touch the disk, so they're tested with plain tables. Integration tests use a temporary `HOME` and local bare repos standing in for GitHub, so every flow runs offline.
