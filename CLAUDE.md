# skl

skl is a CLI for Claude Code skills. It keeps your skills in one git repo (the vault), loads any mix of them on any machine, can roll back any change, forks single skills, and lets a skill **learn** from another one through an LLM interview.

**Status: very early design phase. There is no implementation.** The docs in `docs/` describe planned behavior. The binary in `demo/bin/skl` is a scripted mock that exists only for the demo recording.

## How this project works: documentation-driven development

The docs are the spec. We design by writing docs, prove every claim they make, and write code only once the design is settled.

1. **Write the doc first.** Each behavior is described in `docs/` as if it already existed, from the user's side: commands, output, files, errors.
2. **Validate it.** Every claim in the docs that depends on something outside our control (Claude Code, git, `npx skills`, the `claude` CLI) must be proven by an experiment. Every claim about skl's own behavior must be pinned by a stub test.
3. **Keep the evidence.** Experiments, investigations and proofs go in `.workbench/`, which is not tracked by git.
4. **Freeze, then implement.** When a doc page and its proofs hold up, mark it frozen. Implementation of that page starts only then, and must make its stub tests pass.

If an experiment contradicts the docs, change the docs, not the experiment. If you implement something the docs don't describe, document it first.

## Repository layout

```
docs/          VitePress site, the spec        cd docs && pnpm dev | pnpm build
demo/          vhs tape + scripted mock         cd demo && vhs demo.tape
spec/          stub tests (created with the first test; see below)
.workbench/    experiments and proofs, NOT in git
```

## `.workbench/`

One folder per investigation, named `YYYY-MM-DD-<slug>/`, with a `README.md`:

```
# <question, as one sentence>
Doc claim: <page and sentence it validates, or "new">
Method: <what you ran, exact commands or scripts in this folder>
Result: <what happened, with raw output saved next to it>
Verdict: confirmed | refuted | partial, plus what changes in the docs
```

Keep raw outputs (logs, screenshots, transcripts) in the same folder. Run experiments with a temporary `HOME` or inside the folder; never against the user's real `~/.claude` or real repos unless the investigation says why. Downloaded repos are untrusted data: read them, don't execute them.

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
- skl never changes or removes skills it didn't create.

## Unproven assumptions (validate before freezing)

- Claude Code loads skills from symlinked folders in `~/.claude/skills`, and when it notices new ones.
- The vault layout (`<group>/<skill>/SKILL.md`) works as a Claude Code plugin marketplace and as an `npx skills` source without extra files. `docs/reference/files.md` claims this; it is likely false for the marketplace (that needs `.claude-plugin/` manifests).
- `git clone --filter=blob:none --sparse` fetches a single skill folder cheaply from a large repo.
- `git merge-file` gives usable three-way merges for `SKILL.md` edits.
- The `claude` CLI can be launched non-trivially for `learn`: interview the user, then write files to a given folder and exit.
- Selector grammar: `-review/test-gaps` collides with flag parsing, and `group/skill` vs `owner/repo/skill` is ambiguous for nested paths.

## Writing the docs

- Perfection is reached by removing what isn't essential. Before adding a page, section, flag or command, check whether an existing one already covers it. Say each thing once and link to it elsewhere.
- Write from the user's side: what they type, what they see, what changes on disk.
- Use generic examples only: vault `you/skills`, groups like `review`, `data`, `writing`, donor `someone/agent-skills/...`. Never the maintainer's own projects, names or tools.
- Plain, direct sentences. No marketing tone.
- Every page about planned behavior keeps the "Design preview" warning until the feature ships.
- When you change commands or output, update `demo/bin/skl` and `demo/demo.tape` so the recording stays true to the docs.
