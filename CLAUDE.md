# skl

skl is a CLI for Claude Code skills. It keeps your skills in one git repo (the vault), loads any mix of them on any machine, can roll back any change, and lets a skill **learn** from another one: copied as is if you don't have it, or through an LLM interview if you do.

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
docs/thirdparty/  optional parts that depend on other people's services or tools
demo/          vhs tape + scripted mock         cd demo && vhs demo.tape
spec/          stub tests (created with the first test; see below)
.claude/skills/     the workflow skills
.workbench/         investigations and proofs, NOT in git
.claude/worktrees/  one worktree per card, NOT in git
```

## Stub tests

Stub tests turn doc statements into executable checks before the code exists. They are black-box: they run the `skl` binary against a temporary `HOME` with local bare git repos standing in for GitHub, and assert on exit codes, output and files on disk.

- Go, in `spec/`, one file per doc page (`spec/skill_sets_test.go` ↔ `docs/guide/skill-sets.md`).
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
- `skl learn <peer> [skill] ["<text>"]` (reverses PR #10; there is no `skl fork`). The skill a lesson learns from is its **peer**. No flags for the target or a focus; the optional text goes to Claude as is. One peer per run. The last argument is text if it contains whitespace. The target is `name`, `group/name` or `group/`; left out, it's the vault skill with the peer's name. Names match a folder or frontmatter `name`; a name in two groups stops and asks for `group/name`.
- Target exists → lesson. Missing, no text → verbatim copy, no Claude, recorded as `[source]` (`url`, `commit`) in `.skl-origin.toml`; a renamed copy gets its frontmatter `name` rewritten. Missing, with text → a lesson into an empty skill, recorded as `[[lesson]]` only (no `[source]`, so `--merge` refuses it). A new skill's default group is the peer's owner (`anthropics/skills/pdf` → `anthropics/pdf`), or a local repo's folder name. skl prints `→ copy into …`, `→ lesson into …` or `→ new skill …` and never asks for confirmation; `skl rollback` is the undo. `--as` is for lessons only.
- `skl learn <someone>/@set [@name]` copies a skill set you don't have, rewritten to their repo, with `[source]` in the set file. Never a lesson; an existing set takes changes with `skl upstream @name --merge`.
- `skl learn <skill> ["<text>"]` with only your skill learns again from the last lesson's peer, or a copy's source, taking changes selectively. Skipped changes are declined (`--all` asks again). `skl upstream` only lists changes; `--merge` takes everything. There is no `upstream --learn` and no `learn --again`.
- skl never changes or removes skills it didn't create.
- Any git remote works for every command that takes a repo. `owner/repo` means the default host (GitHub); `//` separates a repo from a path inside it; `#` pins a ref. See `docs/reference/repos.md`.
- Per-machine settings live in `~/.skl/config.toml`, edited by hand (no `skl config` command). Precedence: flag, env var, file, default.
- A **skill set** is a list of selectors (no "profile", no "loaded set"). The loaded one is the **active skill set**; `skl save <name>` stores it as a **named skill set** in `sets/<name>.toml`, one selector per line, loaded with `@name`. It stores selectors, not a resolved list. `save` creates or overwrites (asks first); edit or delete the file by hand. No set subcommands. `sets` and `lessons` can't be group names. "skill set" in prose, "Skill sets" in titles.
- A skill set in someone else's repo is `<repo>/@name` (an `@` at the start of the last path part; `<repo>@name` collides with SSH URLs). Rollback stays global: a generation is a snapshot of the active skill set; there is no `skl rollback @set`.
- Two parts (`review/pr-review`) is your vault; three or more, a host, a URL or a local path is a remote; `//` separates a nested path.
- skl's flags are `--long` only and can go anywhere. Every other word is a selector, so `-review/test-gaps` is a removal (Go's stdlib `flag` can't parse this; skl parses its own args).
- Vault-changing commands commit locally and never push. `skl sync` commits hand edits, merges (never rebases), pushes, and reloads the loaded selectors. A conflict is resolved with Claude (learn engine), by hand (`--continue`), or aborted.
- A skill whose name (folder or frontmatter `name`) is taken by a skill skl didn't create is skipped with a warning. `skl import <name> [group]` copies such a skill into the vault.
- Loaded skills are copies, not links. Each holds `.skl-copy.toml` (source, commit, hash). Changes to a copy are saved back to the vault (three-way, base = the recorded commit) at the start of every command that changes something, as their own generation. People make and improve skills in Claude Code; there is no `skl new`, and `skl import` adopts the folder as skl's copy.
- A changed copy of someone else's skill is never discarded silently: skl asks save to my vault (a copy from the pinned commit plus your changes) / discard / keep. The cache is never written.
- Hand edits in the vault are committed as their own generation before any vault-changing command. A selector that matches nothing warns and is listed in `skl status`.
- A remote reference whose path has no `SKILL.md` and is one word names a skill: skl looks it up in the repo by folder or frontmatter `name` and records the path it found. Two matches stop and list the paths. So `anthropics/skills/pdf` works although the folder is `skills/pdf`.
- `skl open [ref]` replaces `skl star` (no bookmarks, no `stars.toml`). It always prints the web URL of a vault skill, skill set, remote ref or repo, and opens a browser only when stdout is a terminal and a screen exists (no flags; a failed launch exits 0). Known hosts: GitHub, GitLab, Bitbucket, Gitea/Forgejo, sourcehut, Azure; a `[web]` table in `config.toml` names a self-hosted server's software; otherwise the repo root. A copied skill opens its vault page and prints its source. A local repo prints its path. Core, not third party.
- Anything that depends on a third-party service or file format is a removable part: code in `thirdparty/<name>/` behind `internal/ext` (commands, reference rewriters, import sources), docs in `docs/thirdparty/`. Core code and core docs never assume a part exists; core pages only link to it. The parts are skills.sh (`skl search` on its undocumented `/api/search`, skills.sh links as references) and npx skills (`skl import` reads `~/.agents/.skill-lock.json` v3 for the source). skl sends no telemetry.

## Unproven assumptions (validate before freezing)

- Claude Code notices new or changed skill folders in `~/.claude/skills` during a running session.
- The vault layout (`<group>/<skill>/SKILL.md`) works as a Claude Code plugin marketplace without extra files. Likely false (that needs `.claude-plugin/` manifests); the docs no longer claim it.
- `git merge-file` gives usable three-way merges for `SKILL.md` edits.
- The `claude` CLI can be launched non-trivially for `learn`: interview the user, then write files to a given folder and exit.
- A repo whose root is the skill has two parts (`someone/pdf-skill`), which the vault rule claims; `someone/pdf-skill//` reaches it, but where its name comes from is undecided.

## Confirmed assumptions

- Claude Code loads skills from symlinked folders in `~/.claude/skills` and `.claude/skills`, and reads `SKILL.md` from the target (Claude Code skills docs, 2026-10-21). Issue #7. skl now loads copies, not links (issue #15), so this no longer matters.
- Fetching one skill folder with `git clone --depth 1 --filter=blob:none --sparse` works on GitHub, GitLab, Bitbucket, Gitea, Forgejo and local `file://` repos, and older commits can be fetched by sha on demand. Issue #1.
- The vault layout works as an `npx skills` source without extra files (skills@1.7.4, through its recursive fallback; `sets/` and `lessons/` are ignored). Issue #20.

## Writing the docs

- Perfection is reached by removing what isn't essential. Before adding a page, section, flag or command, check whether an existing one already covers it. Say each thing once and link to it elsewhere.
- Write from the user's side: what they type, what they see, what changes on disk.
- Use generic examples only: vault `you/skills`, groups like `review`, `data`, `writing`, peer `someone/agent-skills/...`. Never the maintainer's own projects, names or tools.
- Plain, direct sentences. No marketing tone.
- Order pages and sections by value to the user: what they get first, daily use next, edge cases and mechanism last. Every guide page opens with its goal in one sentence.
- Every page about planned behavior keeps the "Design preview" warning until the feature ships.
- When you change commands or output, update `demo/bin/skl` and `demo/demo.tape` so the recording stays true to the docs.
