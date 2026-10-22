# Architecture

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

## Principles

- **Git is the backend.** Your vault is a git repo, and `skl learn` reads other people's skills straight from theirs. Any git host works, and there's no server.
- **Copies, saved back.** skl loads a skill by copying it into the target folder, and records where each copy came from. Changes made to a copy are saved back to the vault, so any tool can edit a loaded skill.
- **Everything you load is in your vault.** Other repos are only read, by `skl learn` and by `skl sync`'s update check. Copies and lessons are written into your vault.
- **Nothing is lost.** Every change is a generation, and every content change is a commit.

## Loading pipeline

1. **Resolve.** Turn selectors into a flat list of vault skills, in order.
2. **Plan.** Diff against what's loaded now. `--dry-run` stops here.
3. **Apply.** Write the new copies in a staging folder, swap them in, then save a generation.

Loading never touches the network. If the plan is empty, nothing is applied and no generation is made.

If a step fails, the previous set stays in place. A skill whose name is taken by a skill skl didn't create is dropped at the plan step, with a warning.

## Saving back

Each copy holds `.skl-copy.toml`: its source, the commit it was copied from, and the git tree hash of the folder as copied, the same kind of hash `.skl-origin.toml` uses. At the start of every command that changes something, skl hashes each copy and merges each folder that changed into the vault with `git merge-tree`, using the recorded commit as the base. `git merge-tree` sees added, deleted and renamed files, which a file-by-file merge misses. skl commits the result as a generation of its own. If both sides changed the same lines, or one side deleted a file the other changed, it uses the sync conflict flow. Every loaded skill comes from the vault, so there is nowhere else to save to.

`.skl-copy.toml` never enters the vault, and `.skl-origin.toml` stays in the vault: it isn't copied into the loaded folder.

Only files inside a copy's folder are read. Anything next to it, such as skill-creator's `<skill>-workspace/`, is ignored.

## Generations

A generation is a snapshot of what's loaded in each scope plus the vault commit, kept in `~/.skl/generations/`. Generations are local, so their numbers differ from machine to machine.

- A command that changes something makes one generation and prints it last.
- If it first saves changes to loaded skills or hand edits in the vault, that save is a generation of its own, made before the command's. `skl history` lists both, so `skl rollback` undoes the command and keeps your edits.
- A command that changes nothing makes no generation and ends with `✓ nothing to change`.

What's loaded in a repo's project scope (`./.claude/skills`) is kept in `state.json`, keyed by the repo's top-level path. A generation records the user scope and the project scope of the repo it ran in. `skl rollback` and the reload step of `skl sync` act on the user scope and on the current repo's project scope only. Other repos' `.claude/skills` are never touched.

## Safety

One skl runs at a time on a machine, and Ctrl-C finishes or undoes the current step. The exit codes are in [Commands](../reference/commands).

- **The lock.** Every command that changes something takes `~/.skl/lock`. A second run stops with `✗ another skl is running (pid N)`. `status`, `history`, `open` and `help` don't need the lock.
- **Ctrl-C at a question** is the last answer: no, abort or discard.
- **Ctrl-C while applying** finishes or reverts the staging swap, so a scope is never half loaded, then exits with 130.
- **Ctrl-C during a lesson** discards the staging folder. Nothing lands.
- **Ctrl-C during a sync merge** aborts the merge.
- **A merge in progress.** While a sync or save-back conflict waits to be resolved by hand, only `status`, `history`, `open`, `help` and `skl sync --continue` or `--abort` run. Every other command stops with `✗ a sync is in progress · skl sync --continue`. A save-back conflict resolved by hand also finishes with `skl sync --continue`.

## Sync

1. **Save back** changed copies, then **commit** hand edits in `~/.skl/vault`.
2. **Merge** the remote with `git fetch` and `git merge`. Never rebase.
3. **Push.**
4. **Reload** by running the loading pipeline on the selectors already loaded.
5. **Check** each source and peer: one `git ls-remote` per repo, then a blobless fetch of the new tip to compare trees. The result is kept in `state.json` for `skl status`. Nothing is applied.

Loaded copies change only after the merge lands, so a conflict never reaches Claude Code. On a conflict, step 2 waits for the user: the learn engine resolves it, the user edits the markers and runs `skl sync --continue`, or `skl sync --abort` undoes it.

## Fetching from remotes

skl runs git and behaves the same on every host.

- A skill folder is fetched with `git clone --depth 1 --filter=blob:none --sparse`, then `git sparse-checkout set <path>`. On a large repo that's a few hundred KB instead of the full history. Servers without filter support send everything: slower, but still correct.
- Local repos are cloned through `file://`, because git ignores filters for plain paths.
- Merge bases for a lesson are fetched by commit, only when needed.
- The cache lives at `~/.skl/cache/<host>/<path>`. Only `learn` and the update check read it; nothing loaded points into it, so deleting it costs one fetch.
- Git runs with `GIT_TERMINAL_PROMPT=0` and `GIT_SSH_COMMAND="ssh -o BatchMode=yes"`, so a missing credential or an unknown host key fails at once instead of waiting for input.
- The update check reads at most 8 repos at once, and each git call times out after 30 seconds. A source that can't be reached is a `!` line, and the check goes on with the others.
- `SKL_DEBUG=1` prints each git and `claude` command on stderr before it runs.

## Learn engine

skl handles files, git and generations. It copies a skill you don't have, and updates a copy you never changed, without Claude.

**Update.** `.skl-origin.toml` records `tree`, the tree hash of the author's folder at `commit` as skl landed it (with a new `name` or an added license file). After saving back, skl compares it with the vault folder:

1. If the author's tree at the new tip equals the one at `commit`, or equals yours, the skill is up to date: only `commit` moves.
2. If your folder equals `tree`, you never changed it: skl replaces the folder with the author's version and records the new `commit` and `tree`. No Claude.
3. Otherwise it runs a lesson. A skill with only lessons, and any `learn` with text, always takes this path.

After a lesson, `tree` becomes the author's tree at the new commit, so a lesson that restores the author's version leaves a skill that updates without Claude again.

**License.** skl looks for a `LICENSE*` or `COPYING*` in the skill's folder, then at the repo root, then for the `license` field in `SKILL.md`, and recognizes the text. No license, an unknown one, or one that isn't permissive and isn't the vault's own makes skl ask (copies and updates) or puts the question in the lesson brief. skl applies no other rule.

**Lesson.** For a lesson, Claude Code does the analysis and the interview. skl prepares a workspace with `mine/`, `peer/` and a brief holding your text, then runs `claude` there with a bundled lesson skill. For a new skill built from text, `mine/` is empty. When you learn again from the same peer or a copy's source, it also adds `base/`, the version you copied or last learned from, plus `merged/` and `conflicts.txt`, the result of `git merge-tree` on the three, as a starting point. `license/` holds the peer's license and the vault's. A sync conflict uses the same engine with a bundled merge skill, and `base/`, `mine/` and `theirs/` for each conflicted file. Claude writes the result to `result/`, and skl then shows the diff and lands it.

How the interview reaches you isn't settled. Either `claude` runs interactively in the workspace and asks in Claude Code's own interface, or skl asks the questions Claude writes and passes the answers back. A spike before v0.3 decides it, because it rests on an unproven assumption: that `claude` can be launched to interview the user, write files to a given folder and exit. The examples in the guide show what is asked, not how it looks.

## Import

`skl import` copies a folder from `~/.claude/skills` into the vault and adopts it as a loaded skill. If the folder is a link, as `npx skills` may leave it, skl replaces the link with a real copy and says so (`◇ replaced link ~/.claude/skills/pdf with a copy`), so `.skl-copy.toml` is never written into another tool's folder. When an import source part finds where the skill came from, skl records it as `[source]`, with `tree` set to the folder as imported, and runs the same license check as `skl learn`.

## Guarantees

- Skills skl didn't create are never changed or removed. If one holds a name skl needs, skl skips its own skill.
- Every change can be rolled back, lessons included.
- A change made to a loaded copy is never discarded without asking.
- A skill from another repo enters your vault only through `skl learn`, which shows it, its license and its scripts and asks first, or through `skl import` of a folder already on this machine, which runs the same license check when it knows the source. Every copy records the commit and tree it came from.
- Without skl, the vault is still an ordinary git repo of skills.
- skl sends no telemetry and never checks for its own updates.

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
    ├── learn/      # copy, update paths, workspace,
    │               # claude engine, land
    ├── license/    # find and recognize a license (pure)
    └── ext/        # hooks for third-party parts
thirdparty/
├── skillssh/       # skl search, skills.sh links
├── npxskills/      # import source from the npx skills lock
└── gh/             # create a missing vault repo, protocol
```

## Third-party parts

Code that depends on someone else's service, tool or file format lives in `thirdparty/<name>/`, never in `internal/`. Core offers four hooks in `internal/ext` and knows nothing about what plugs into them:

```go
// a new subcommand
func RegisterCommand(c Command)
// runs before a repo reference is parsed
func RegisterRefRewriter(f func(ref string) (string, bool))
// where a skill being imported came from
func RegisterImportSource(f func(string) (ImportSource, bool))
// creates a vault repo that doesn't exist yet
func RegisterRepoCreator(c RepoCreator)
```

Each part registers itself in `init()`, and `cmd/skl/thirdparty.go` imports it with one line. Deleting the package and that line removes the part: everything else builds and passes its tests. Its settings are a table in `config.toml` that only the part reads; core ignores tables it doesn't own. A part may also supply the default of a core setting: the `gh` part takes `protocol` from `gh`'s own config. Without a repo creator, `skl init` on a missing repo stops with `✗ you/skills doesn't exist · create it, then skl init you/skills`; without the `gh` part, `protocol` defaults to `https`. A part's failure stays inside its own command. Apart from git, only the skills.sh part, and the `gh` CLI that the `gh` part runs, reach the network.

`selector` and `resolve` never touch the disk, so they're tested with plain tables. Integration tests use a temporary `HOME` and local bare repos standing in for GitHub, so every flow runs offline.
