# skl

skl is a CLI for Claude Code skills. It keeps your skills in one git repo (the vault), loads any mix of them on any machine, can roll back any change, and lets a skill **learn** from another one: copied as is if you don't have it, or through an LLM interview if you do. `learn` is the only way a skill from elsewhere gets in.

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
- Target exists → lesson. Missing, no text → verbatim copy, no Claude, recorded as `[source]` (`url`, `commit`, `tree`, `ref` if pinned, `license`) in `.skl-origin.toml`; a renamed copy gets its frontmatter `name` rewritten. Missing, with text → a lesson into an empty skill, recorded as `[[lesson]]` only (no `[source]`). A new skill's default group is the peer's owner (`someone/agent-skills/pdf` → `someone/pdf`), or a local repo's folder name; a repo-root skill (`someone/pdf-skill//`) takes its frontmatter `name`, else the repo name. skl prints `→ copy into …`, `→ lesson into …` or `→ new skill …`; `skl rollback` is the undo. A learned new skill is loaded in the same generation.
- **Reversed by the user (2026-10-22, #29):** PR #22's "never asks for confirmation". Before someone else's new skill, or an update that changes their scripts (any changed non-`*.md` file or mode) or raises a license issue, skl shows the description, license and scripts and asks `Add it?` once; `--yes` answers. **Dropped (coordinator, #29):** `--as` (the landing prompt keeps "save as") and `--all`.
- `skl learn <someone>/@set [@name]` copies their set and every skill it names into the owner group (`someone/…`); the set file lists the copies, and `sets/<name>.origin.toml` holds `[source]` and `selected`, the lines skl last wrote (review C11f, 2026-10-22). Two of their skills with the same folder name: the later lands as `<group>-<name>` with its frontmatter `name` rewritten, printed. A vault skill at the target that isn't a copy of theirs is skipped with a warning, never a lesson. Learning it again copies new skills, updates each one, drops removed ones from the set file (the vault copies stay) and keeps your own lines. Never a lesson. (Replaces "rewritten to their repo", #29.)
- `skl learn <skill> ["<text>"]` with only your skill takes changes from the last lesson's peer, or a copy's source. skl picks the path from `tree` (their folder at `commit` as landed): their tree didn't change → up to date; your folder equals `tree` → update, the folder replaced, no Claude; otherwise, or with any text → a lesson over base, peer and mine, with `git merge-tree`'s result as a starting point. An edited skill always updates by lesson, even if the text merge is clean; without the `claude` CLI it stops. After a lesson, `tree` becomes theirs at the new commit. "Take theirs" has no flag: say it in the text. `skl learn` with no arguments takes every update; `--dry-run` lists. **Removed by the user (#29):** `skl upstream`, `--merge`, `upstream --continue`. `skl sync` checks sources (never applies); `skl status` shows `update available` offline.
- Tags and commits pinned with `#` never update; branches are followed.
- Licenses (user, #29; a docs design, not legal advice): the vault has its own `LICENSE`, written by `skl init` from config `license` (default MIT). skl finds a skill's license (folder, repo root, frontmatter) and copies a root license into the copy. No license, an unknown one, or a non-permissive one that isn't the vault's is an issue: skl asks (keep their license and notice / no / as it is), or Claude asks in a lesson. No other rule.
- skl never changes or removes skills it didn't create.
- Any git remote works for every command that takes a repo. `owner/repo` means the default host (GitHub); `//` separates a repo from a path inside it; `#` pins a ref. See `docs/reference/repos.md`.
- Per-machine settings live in `~/.skl/config.toml`, edited by hand (no `skl config` command); the file is optional. Precedence: flag, env var, file, default. There is no `vault` key: the vault is the clone at `~/.skl/vault` and its `origin` (review C8). Env vars beyond `SKL_<KEY>`: `SKL_HOME`, `SKL_DEBUG`, `NO_COLOR`, `BROWSER`.
- A **skill set** is a list of selectors (no "profile", no "loaded set"). The loaded one is the **active skill set**; `skl save <name>` stores it as a **named skill set** in `sets/<name>.toml`, one selector per line, loaded with `@name`. It stores selectors, not a resolved list. `save` creates or overwrites (asks first); edit or delete the file by hand. No set subcommands. `sets`, `lessons` and `h` can't be group names. "skill set" in prose, "Skill sets" in titles.
- A skill set in someone else's repo is `<repo>/@name` (an `@` at the start of the last path part; `<repo>@name` collides with SSH URLs), as a `learn` argument only. Rollback stays global: a generation is a snapshot of the active skill set; there is no `skl rollback @set`.
- Two parts (`review/pr-review`) is your vault; three or more, a host, a URL or a local path is a remote; `//` separates a nested path. **Reversed by the user (#29):** selectors are vault-only; `use`/`add`/`drop` and set files reject a remote reference with a hint to `skl learn` it. There are no read-only remote skills. Grammar corners (review C14, `docs/reference/repos.md` "Grammar"): `-` removes groups and sets too (`-review`, `-@base`); names have no dots; an owner name that's reserved or invalid gets the group `<owner>-skills`; a one-word name that's a skill but no group warns `! pdf   matches no group · skl use someone/pdf`. Sources and peers are stored as canonical URLs: HTTPS for shorthand, as typed otherwise, trailing `.git` dropped.
- skl's flags are `--long` only and can go anywhere, as `--x v` or `--x=v`; `--` ends flags. Every other word is a selector, so `-review/test-gaps` is a removal (Go's stdlib `flag` can't parse this; skl parses its own args). **The one exception (user, 2026-10-22, D1):** `-h` always means `--help`. Help is `skl help [cmd]`, `--help`/`-h`, and bare `skl` lists the commands; `skl --version` prints the version and build commit.
- Vault-changing commands commit locally and never push (user, #31: no auto-push); `skl status` and a `! N changes not pushed · skl sync` line before the `✓` remind instead. `skl sync` commits hand edits, merges (never rebases), pushes, and reloads the loaded selectors. A conflict is resolved with Claude (learn engine), by hand (`--continue`), or aborted.
- A skill whose name (folder or frontmatter `name`) is taken by a skill skl didn't create is skipped with a warning. `skl import <name>… [--group <g>]` copies such skills into the vault; the group defaults to the vault's owner (`you/skills` → `you/`). `skl init <repo> [selectors]` clones or creates the vault, loads the selectors, and offers to import the skills in `~/.claude/skills` that aren't in a vault (#31).
- Loaded skills are copies, not links. Each holds `.skl-copy.toml` (source, always a vault skill; commit; `tree`, the folder's git tree hash), which is never saved back. Changes to a copy are saved back to the vault (three-way with `git merge-tree`, base = the recorded commit; review X1, X2) at the start of every command that changes something, as their own generation. People make and improve skills in Claude Code; there is no `skl new`, and `skl import` adopts the folder as skl's copy.
- Hand edits in the vault are committed as their own generation before any vault-changing command. A selector that matches nothing warns and is listed in `skl status`.
- A remote reference whose path has no `SKILL.md` and is one word names a skill: skl looks it up in the repo by folder or frontmatter `name` and records the path it found. Two matches stop and list the paths. So `someone/agent-skills/pdf` works although the folder is `skills/pdf`.
- `skl open [ref]` replaces `skl star` (no bookmarks, no `stars.toml`). It always prints the web URL of a vault skill, skill set, remote ref or repo, and opens a browser only when stdout is a terminal and a screen exists (no flags; a failed launch exits 0). Known hosts: GitHub, GitLab, Bitbucket, Gitea/Forgejo, sourcehut, Azure; a `[web]` table in `config.toml` names a self-hosted server's software; otherwise the repo root. A copied skill opens its vault page and prints its source. A local repo prints its path. A two-part argument is a vault skill if one exists, else `owner/repo` on the default host (review X6). Core, not third party.
- One output vocabulary (#31, listed in `docs/reference/commands.md` "Output"): `→` plan, `◇` step, `+ ~ −` loaded/replaced/unloaded, `?`/`›` question (first answer is the default), `!` warning, `✗` error, `✓` result, last; a change ends `✓ generation N · skl rollback to undo`. Every `!` and `✗` line ends with the command to run, after `·`, on the next line if it doesn't fit. In user-facing prose, a skill in `~/.claude/skills` is "loaded", not a "copy"; "copy" is a skill learned without Claude.
- Anything that depends on a third-party service or file format is a removable part: code in `thirdparty/<name>/` behind `internal/ext` (commands, reference rewriters, import sources), docs in `docs/thirdparty/`. Core code and core docs never assume a part exists; core pages only link to it. The parts are skills.sh (`skl search` on its undocumented `/api/search`, skills.sh links as references), npx skills (`skl import` reads `~/.agents/.skill-lock.json` v3 for the source) and gh (user, 2026-10-22, D4: `skl init` creates a missing GitHub vault repo with `gh repo create --private`, and `protocol` defaults from gh's config; without the part, init stops with `✗ you/skills doesn't exist · create it, then skl init you/skills` and `protocol` defaults to `https`). skl sends no telemetry and never checks for its own updates.
- **CLI conventions (docs review, user, 2026-10-22; `docs/reference/commands.md`).**
  - Exit codes: `0` done (warnings, a no-op and `--dry-run` included), `1` error, `2` usage error, `130` Ctrl-C. A warning never changes the exit code; a skipped skill is a warning.
  - Questions: `--yes` takes the first answer of every question. Without a terminal and without `--yes`, a question stops the command before it changes anything. A lesson and a sync conflict resolved with Claude always need a terminal: `learn` stops before Claude runs, `sync` undoes the merge and stops.
  - Output: what you asked for (status, history, the `open` URL, a `--dry-run` plan, `--json`) on stdout; marks and questions on stderr. Color only on a terminal; `NO_COLOR` and `TERM=dumb` turn it off.
  - **D2 (user):** `--json` on `status` and `history` only (format may change until 1.0); `SKL_DEBUG=1` prints each git and `claude` command to stderr. **D3 (user):** `skl completion bash|zsh|fish` completes commands, flags and vault selectors (roadmap v0.4).
  - `--dry-run` works on every command that changes something and prints the plan; `sync --dry-run` fetches but doesn't merge or push. `--scope` applies to `init use add drop save learn rollback` (and `import`); `project` is `.claude/skills` at the git repo root, an error outside a repo.
  - Generations: a command that first saves edits makes two generations (the save, then its own); a no-op makes none and ends `✓ nothing to change`. Generations are local, numbered per machine. A project scope's active skill set lives in `state.json`, keyed by repo; rollback and sync's reload act on the user scope and the current repo's project scope only.
  - Safety: one skl runs at a time (a lock). While a by-hand sync merge is pending, only `status`, `history`, `open`, `help` and `sync --continue|--abort` run. Ctrl-C at a question takes its last answer; Ctrl-C while changing finishes or undoes the current step.
- **Learn and import details (docs review, coordinator calls accepted by the user, 2026-10-22).** A learned new skill joins the active skill set, as `skl add` would. "Scripts" = any non-`*.md` file. `Land it` for a lesson is `› replace  change it  save as …  discard`; a discarded lesson records nothing. Your own vault skill is never copied (`✗ … already in your vault`, with a hint to run a lesson). `skl import` replaces a symlink in `~/.claude/skills` with a real copy, and runs the license check when an import source part finds where the skill came from. How lessons run Claude (Claude Code's own interview UI vs skl asking) is proved by a spike before v0.3.

## Unproven assumptions (validate before freezing)

- Claude Code notices new or changed skill folders in `~/.claude/skills` during a running session.
- The vault layout (`<group>/<skill>/SKILL.md`) works as a Claude Code plugin marketplace without extra files. Likely false (that needs `.claude-plugin/` manifests); the docs no longer claim it.
- The `claude` CLI can be launched non-trivially for `learn`: interview the user, then write files to a given folder and exit.

## Confirmed assumptions

- `git merge-file` merges `SKILL.md` edits cleanly only when they are at least a line apart, and a clean merge can still contradict itself; a folder modify/delete needs `git merge-tree` (#29). So merges only feed a lesson, and save-back merges a folder with `git merge-tree` (docs review X1, 2026-10-22).
- A recorded git tree hash tells exactly whether a copy is unchanged (edits, undone edits, mode bits, CRLF), offline, in a few ms (#29).

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
