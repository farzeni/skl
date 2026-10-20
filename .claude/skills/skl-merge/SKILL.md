---
name: skl-merge
description: >-
  Step 3 of the skl workflow (skl-investigate → skl-implement → skl-merge). Merge an approved skl
  PR, confirm its card reached Done on the SKL Board, remove the worktree and branches, and mark
  the investigation delivered. Use when the user says merge, ship, land or "you can merge it"
  about an skl PR.
argument-hint: "<PR number>"
---

# skl-merge

Step 3 of 3. It starts from an **open PR** left by [skl-implement](../skl-implement/SKILL.md) and ends with the card **Done**, no leftover worktree, and the investigation from [skl-investigate](../skl-investigate/SKILL.md) marked **delivered**. The board ids are in `CLAUDE.md` under "Board".

Input: `$ARGUMENTS`, the PR number.

## 0. Check the state

- **The user approved merging this PR, in this conversation.** Approval for one PR doesn't carry over to another.
- **The PR is open and mergeable**, and `gh pr checks` passes, if there are checks.
- **The proofs are current.** If the PR changed after its proofs were made, run `/testit` again before merging.

If anything fails, report it and stop.

## 1. Merge

```sh
gh pr merge <pr> --repo farzeni/skl --squash --delete-branch
```

## 2. Card

The board moves the card to **Done** when the PR closes its issue. Check with `gh project item-list`. If it didn't move, set it to Done.

## 3. Clean up

From the main checkout:

```sh
git worktree remove .claude/worktrees/<issue>-<slug>
git branch -D <issue>-<slug>
git fetch -q --prune && git pull -q --ff-only
```

## 4. Close the loop

1. In the investigation README, set `Status: delivered` and add the merge commit to its `Delivery` section.
2. Report to the user:
   - the merge commit;
   - the card's status;
   - the PR's follow-ups;
   - anything under "Left over" that still needs them, such as real resources created during tests.
3. Each follow-up starts again at skl-investigate. A trivial one may go straight to skl-implement if the user agrees.
