# Architecture

## Principles

- **Git is the backend.** Your vault is a git repo, and other people's skills are read straight from theirs. Any git host works, and there's no server.
- **Links, not copies.** skl loads a skill by symlinking it into the target folder, and records every link it creates.
- **The vault is the only thing you edit.** Other repos are read-only. Forks and lessons are written into your vault.
- **Nothing is lost.** Every change is a generation, and every content change is a commit.

## Loading pipeline

1. **Resolve.** Turn selectors into a flat list of skills, in order.
2. **Fetch.** Sparse-fetch any remote skill folder that isn't cached yet.
3. **Plan.** Diff against what's linked now. `--dry-run` stops here.
4. **Apply.** Create the new links in a staging folder, swap them in, then save a generation.

If a step fails, the previous set stays in place.

## Fetching from remotes

skl runs git and behaves the same on every host.

- A skill folder is fetched with `git clone --depth 1 --filter=blob:none --sparse`, then `git sparse-checkout set <path>`. On a large repo that's a few hundred KB instead of the full history. Servers without filter support send everything: slower, but still correct.
- Local repos are cloned through `file://`, because git ignores filters for plain paths.
- Merge bases for `skl upstream` are fetched by commit, only when needed.
- The cache lives at `~/.skl/cache/<host>/<path>`.
- Git runs with `GIT_TERMINAL_PROMPT=0` and `GIT_SSH_COMMAND="ssh -o BatchMode=yes"`, so a missing credential or an unknown host key fails at once instead of waiting for input.

## Learn engine

skl handles files, git and generations. Claude Code does the analysis and the interview. skl prepares a workspace with `mine/`, `donor/` and a brief, then runs `claude` there with a bundled lesson skill. Claude asks its questions in its own interview UI and writes the result to `result/`. skl then shows the diff and lands it.

## Guarantees

- Skills skl didn't create are never changed or removed.
- Every change can be rolled back, lessons included.
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
    ├── vault/      # groups, profiles, stars
    ├── source/     # remote repos, cache
    ├── link/       # plan, apply, generations
    ├── fork/       # origin, upstream merge
    └── learn/      # workspace, claude engine, land
```

`selector` and `resolve` never touch the disk, so they're tested with plain tables. Integration tests use a temporary `HOME` and local bare repos standing in for GitHub, so every flow runs offline.
