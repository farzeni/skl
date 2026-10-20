# skl

skl is a CLI for Claude Code skills. It keeps your skills in one git repo (the vault), loads any mix of them on any machine, can roll back any change, forks single skills, and lets a skill **learn** from another one through an LLM interview.

**Status: very early design phase. There is no implementation.** The docs in `docs/` describe planned behavior. The binary in `demo/bin/skl` is a scripted mock that exists only for the demo recording.

## How we work

Documentation-driven development: the docs in `docs/` are the spec. Every behavior is written there first, from the user's side, and every claim is proven before any code is written. If an experiment contradicts the docs, the docs change. If something gets built that the docs don't describe, it gets documented first.

Every change goes through three project skills, in order. Each one picks up the state the previous one left:

| Skill | Starts from | Leaves |
|---|---|---|
| `/skl-investigate` | a question or a doc claim | a closed investigation in `.workbench/` with findings, decisions and a plan |
| `/skl-implement` | a closed investigation | a card In Progress, a worktree, a change verified with `/testit`, an open PR |
| `/skl-merge` | an approved PR | the PR merged, the card Done, the worktree removed, the investigation delivered |

Never commit to `main` directly.

## Board

Work is tracked on the SKL Board, https://github.com/users/farzeni/projects/1. Each card is an issue in `farzeni/skl`, and the board adds new issues on its own. Cards are In Progress from the start of work until the PR merges, then Done. `gh` needs the `project` scope.

```sh
gh project item-list 1 --owner farzeni --format json      # find a card's item id
gh project item-edit --project-id PVT_kwHOABQPns4Bn-yc --id <item-id> \
  --field-id PVTSSF_lAHOABQPns4Bn-yczhmg69s --single-select-option-id <option-id>
```

Option ids: Todo `f75ad846`, In Progress `47fc9ee4`, Done `98236657`.

## Repository layout

```
docs/          VitePress site, the spec        cd docs && pnpm dev | pnpm build
demo/          vhs tape + scripted mock         cd demo && vhs demo.tape
spec/          stub tests (created with the first test; see below)
.claude/skills/     the workflow skills
.workbench/         investigations and proofs, NOT in git
.claude/worktrees/  one worktree per card, NOT in git
```

## Stub tests

Stub tests turn doc statements into executable checks before the code exists. They are black-box: they run the `skl` binary against a temporary `HOME` with local bare git repos standing in for GitHub, and assert on exit codes, output and files on disk.

- Go, in `spec/`, one file per doc page (`spec/loading_test.go` ↔ `docs/guide/loading.md`).
- Each test names the doc sentence it checks in a comment.
- Until the feature exists the test calls `t.Skip("pending: <page>")`. Remove the skip when implementing.
- Pure logic with a clear contract (selector parsing, resolution order) gets table tests as soon as the grammar is frozen.

## Decisions already made

Don't reopen these without a reason from an experiment.

- Name `skl`. The combine concept is `learn`; one run is a **lesson** (`[[lesson]]` in `.skl-origin.toml`).
- Default scope is user (`~/.claude/skills`); `--scope project` for `./.claude/skills`.
- Every change creates a **generation**; `skl rollback` never rewrites git history.
- Git is the only backend; no server. One Go binary that shells out to the git CLI.
- `learn` runs the user's `claude` CLI in a prepared workspace. Claude writes to staging; only skl commits.
- `skl learn <skill> <peer> ["<text>"]`: the skill a lesson learns from is its **peer**. No flags for the target or a focus; the optional text goes to Claude as is. One peer per run.
- `skl upstream <skill> --learn ["<text>"]` takes changes selectively from a fork's source or a lesson's peer. Skipped changes are declined (`--all` shows them again). `--merge` takes everything. There is no `learn --again`.
- skl never changes or removes skills it didn't create.
- Any git remote works for every command that takes a repo. `owner/repo` means the default host (GitHub); `//` separates a repo from a path inside it; `#` pins a ref. See `docs/reference/repos.md`.
- Per-machine settings live in `~/.skl/config.toml`, edited by hand (no `skl config` command). Precedence: flag, env var, file, default.

## Unproven assumptions (validate before freezing)

- Claude Code loads skills from symlinked folders in `~/.claude/skills`, and when it notices new ones.
- The vault layout (`<group>/<skill>/SKILL.md`) works as a Claude Code plugin marketplace and as an `npx skills` source without extra files. `docs/reference/files.md` claims this; it is likely false for the marketplace (that needs `.claude-plugin/` manifests).
- `git merge-file` gives usable three-way merges for `SKILL.md` edits.
- The `claude` CLI can be launched non-trivially for `learn`: interview the user, then write files to a given folder and exit.
- Selector grammar: `-review/test-gaps` collides with flag parsing, and `group/skill` vs `owner/repo/skill` is ambiguous for nested paths.

## Confirmed assumptions

- Fetching one skill folder with `git clone --depth 1 --filter=blob:none --sparse` works on GitHub, GitLab, Bitbucket, Gitea, Forgejo and local `file://` repos, and older commits can be fetched by sha on demand. Issue #1.

## Writing the docs

- Perfection is reached by removing what isn't essential. Before adding a page, section, flag or command, check whether an existing one already covers it. Say each thing once and link to it elsewhere.
- Write from the user's side: what they type, what they see, what changes on disk.
- Use generic examples only: vault `you/skills`, groups like `review`, `data`, `writing`, peer `someone/agent-skills/...`. Never the maintainer's own projects, names or tools.
- Plain, direct sentences. No marketing tone.
- Every page about planned behavior keeps the "Design preview" warning until the feature ships.
- When you change commands or output, update `demo/bin/skl` and `demo/demo.tape` so the recording stays true to the docs.
