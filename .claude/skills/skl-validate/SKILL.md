---
name: skl-validate
description: >-
  The validation step of the skl workflow for code cards, between skl-implement and skl-merge.
  Check someone else's skl PR against its card and the docs on the PR head: every acceptance
  criterion and doc sentence, make check and the spec scripts rerun, proofs saved. Answer GREEN
  or a numbered list of findings, each with file:line and the doc sentence. Never edits the PR.
  Use when asked to validate, check or verify an skl PR for a card, or when a coordinator starts
  you as a validator (for example skl/build/val-<card>).
argument-hint: "<PR number>"
---

# skl-validate

The step between [skl-implement](../skl-implement/SKILL.md) (code mode) and [skl-merge](../skl-merge/SKILL.md). It starts from an **open PR** and its card, and ends with a verdict on the PR's current head: **GREEN**, or **numbered findings**. Your GREEN is what lets the implementer merge without the user's "merge", so check as if you were the user.

Input: `$ARGUMENTS`, the PR number. The **implementer** is the session that opened the PR. The **coordinator** is the session that started you (in the build, SKL). Without one, the user is the coordinator.

## Rules

- **Read-only.** Never commit, push, or edit the PR's files. A fix you could make is a finding.
- **Separate.** Never validate a PR you wrote. Don't trust the implementer's proofs. Reproduce them.
- **The docs are the spec.** The code must do what the doc sentence says: the same marks, wording, files and exit code. If you think the doc is wrong, it's still a finding. Also tell the coordinator, since a docs PR comes first.
- **Isolate.** Run `skl` only against a temporary `HOME`. Never touch the real `~/.claude`, `~/.skl` or other configs. No logins and no model calls.

## 0. Check the state

- The PR is open and its body has `Closes #<card>`.
- Read `CLAUDE.md`, `CONTRIBUTING.md`, the card, and every doc page and section the card names, all the way through. Then read the PR body and its diff.

## 1. Check out the head

From the main checkout:

```sh
sha=$(gh pr view <pr> --repo farzeni/skl --json headRefOid -q .headRefOid)
git fetch -q origin "pull/<pr>/head"
git worktree add --detach .claude/worktrees/val-<pr> "$sha"   # later rounds: git -C .claude/worktrees/val-<pr> checkout -q --detach <sha>
```

Every check below runs in that worktree, at that sha.

## 2. Check

Write the results to `.workbench/YYYY-MM-DD-card-<card>/validation/round-<k>.md` in the main checkout, in the card's folder the implementer made (create it with today's date if there is none). Save raw outputs next to it. Each item gets a command and its output, not an opinion.

1. **Acceptance.** Each criterion in the card's `Acceptance`: met or not, with the evidence.
2. **Doc sentences.** For each sentence under the card's `Docs implemented`:
   - a script block in `spec/testdata/` preceded by `# "<sentence>"`, or, for a pure package with no command yet, a table test case naming it;
   - the block checks what the sentence says (read it; don't trust the comment);
   - it passes and isn't skipped, unless the PR lists the sentence as left for later and the card allows it.
3. **Checks.** Run `make check` on the head. Also run `go test ./spec/...` on its own, to see each script's result.
4. **Behavior.** Run `/testit` with the acceptance criteria and doc sentences as its claims. Build `skl`, run it against a temporary `HOME`, and try at least one case from the docs that the scripts don't cover: no terminal, `--dry-run`, a bad argument (exit 2), a re-run that should be a no-op.
5. **Scope.** `git diff --stat origin/main...$sha` touches only the card's `Packages and files touched`, and the code follows the card's `Design`. No doc page changes its meaning, and no `CLAUDE.md` decision is reversed, without the user.
6. **Conventions.** The change follows `CONTRIBUTING.md`: errors, context, output only through `ui`, no global state, the right testing layer. Each new dependency is justified in the PR.
7. **CI.** `gh pr checks <pr> --repo farzeni/skl` is green on this sha. If it's still running, wait with `--watch`.

## 3. Verdict

A finding is **blocking** when an acceptance criterion or a doc sentence isn't met, a check fails, the scope grew, or a convention is broken. Anything else is a **note**, a follow-up that doesn't block.

- **GREEN** only when nothing blocks:

  ```
  GREEN at <sha> · round <k>
  Checked: <acceptance, sentences, make check, spec, testit, scope, CI, in one line each>
  Notes: <follow-ups, or none>
  Proofs: .workbench/YYYY-MM-DD-card-<card>/validation/round-<k>.md
  ```

- Otherwise, findings, numbered across rounds so they can be referred to:

  ```
  Findings at <sha> · round <k>
  1. internal/app/use.go:42: prints "✓ done" instead of "✓ nothing to change".
     Doc: "A command that changes nothing makes no generation and ends with `✓ nothing to change`." (docs/reference/commands.md "Output")
     Evidence: validation/round-1/rerun.txt
  ```

  Each finding has a file:line, the doc sentence or acceptance criterion with its page, and the evidence.

Post the verdict as a PR comment (`gh pr comment <pr> --repo farzeni/skl --body-file <file>`), so skl-merge can check it later. In a swarm, also send it to the implementer.

## 4. Next round

When the implementer replies with a new head, go back to step 1 with the new sha.

- Rerun every check, not only the diff, and say for each earlier finding whether it's fixed.
- If the implementer disagrees with evidence, either drop the finding or keep it and say why.
- After round 3 without GREEN, stop and tell the coordinator: the findings still open and both sides' evidence.

## 5. Clean up

After GREEN or a stop, remove your worktree with `git worktree remove .claude/worktrees/val-<pr>`. Report to the coordinator in one line: `GREEN at <sha> · PR #<pr>`, or the stop.
