---
name: skl-merge
description: >-
  Step 3 of the skl workflow (skl-investigate → skl-implement → skl-merge). Merge an approved skl
  PR, confirm its card reached Done on the SKL Board, remove the worktree and branches, and mark
  the investigation delivered. For a code card, the validator's GREEN on the current PR head
  replaces the user's approval, and CI must be green. Use when the user says merge, ship, land or
  "you can merge it" about an skl PR, or when skl-validate gave GREEN on your code card's PR.
argument-hint: "<PR number>"
---

# skl-merge

Step 3 of 3. It starts from an **open PR** left by [skl-implement](../skl-implement/SKILL.md) and ends with the card **Done**, no leftover worktree, and, in docs mode, the investigation from [skl-investigate](../skl-investigate/SKILL.md) marked **delivered**. The board ids are in `CLAUDE.md` under "Board".

Input: `$ARGUMENTS`, the PR number.

## 0. Check the state

- **The merge is approved**, in one of two ways:
  - **Docs mode:** the user approved merging this PR, in this conversation. Approval for one PR doesn't carry over to another.
  - **Code card** (see skl-implement): the latest PR comment headed `skl-validate · round <k>` (`gh pr view <pr> --repo farzeni/skl --comments`), posted by [skl-validate](../skl-validate/SKILL.md), says `GREEN at <sha>`, and `<sha>` is the PR's current head (`gh pr view <pr> --repo farzeni/skl --json headRefOid`). A push after GREEN voids it. The user's "merge" isn't needed.
- **The PR is open and mergeable.**
- **CI is green.** `gh pr checks <pr> --repo farzeni/skl` passes. A code card's PR must have checks: `no checks reported` is a stop, unless the coordinator waived it in a PR comment (only #73, which landed before CI existed). In docs mode, a PR with no checks may merge.
- **The proofs are current.** In docs mode, if the PR changed after its proofs were made, run `/testit` again. For a code card, GREEN on the head covers it.

If anything fails, report it to the user or the coordinator and stop.

## 1. Merge

```sh
gh pr merge <pr> --repo farzeni/skl --squash --delete-branch --match-head-commit <head sha>
```

`--match-head-commit` refuses to merge if the head moved since you checked it.

## 2. Card

The board moves the card to **Done** when the PR closes its issue. Check with `gh project item-list`. If it didn't move, set it to Done.

## 3. Clean up

Stop anything you started from the worktree, such as `pnpm preview`. Then, from the main checkout (leave the worktree first):

```sh
git worktree remove .claude/worktrees/<issue>-<slug>
git branch -D <issue>-<slug>
git fetch -q --prune
git pull -q --ff-only   # skip when other agents share this checkout; the coordinator updates main
```

## 4. Close the loop

1. **Docs mode:** in the investigation README, set `Status: delivered` and add the merge commit to its `Delivery` section.
2. Report to the user, or to the coordinator for a code card, as `merged <commit> · <what's in it> · <follow-ups>`:
   - the merge commit;
   - the card's status;
   - the PR's follow-ups, and the validator's notes;
   - anything under "Left over" that still needs them, such as real resources created during tests.
3. Each follow-up starts again at skl-investigate, or becomes a card in Backlog for the coordinator. A trivial one may go straight to skl-implement if the user agrees.
