---
name: skl-implement
description: >-
  Step 2 of the skl workflow (skl-investigate → skl-implement → skl-merge). Turn a closed
  .workbench investigation into a change: create its card on the SKL Board, work in a worktree,
  apply the plan, verify it with /testit (for docs: the site builds and every changed page
  renders), and open a PR that closes the card. Use when the user says to apply, implement,
  proceed with or "do the real change" in the skl project.
argument-hint: "<investigation slug>"
---

# skl-implement

Step 2 of 3. It starts from a **closed investigation** left by [skl-investigate](../skl-investigate/SKILL.md) and ends with an **open PR** for [skl-merge](../skl-merge/SKILL.md). The board ids and the commands for setting a card's status are in `CLAUDE.md` under "Board".

Input: `$ARGUMENTS`, the slug of a folder in `.workbench/`.

## 0. Check the state

- **The investigation has `Status: closed`.** If it's still open, say what's missing (open questions, untested points) and go back to skl-investigate.
- **The only exception** is a request the user fully specified and chose not to investigate. Then the request is the plan.

## 1. Card

`.workbench/` isn't in git, so the issue must stand on its own. Its body holds:
- the question;
- the findings, with key evidence;
- the decisions;
- the plan.

```sh
gh issue create --repo farzeni/skl --title "<what will change>" --body-file <file>
```

The board adds new issues on its own. Find the card's item id with `gh project item-list`, then set it to **In Progress**.

## 2. Worktree

```sh
git fetch -q && git worktree add .claude/worktrees/<issue>-<slug> -b <issue>-<slug> origin/main
cd .claude/worktrees/<issue>-<slug>/docs && pnpm install --frozen-lockfile
```

All edits happen in the worktree. Never on `main`.

## 3. Apply the plan, and only the plan

- Follow the writing rules in `CLAUDE.md`.
- Anything wrong you notice outside the plan goes on a follow-up list for the PR. Don't fix it here.
- If the plan turns out to be wrong, stop and go back to skl-investigate.

## 4. Verify with /testit

Run `/testit` on the branch. Its claims are the plan items and the decisions. For this project they mean:

- **Docs**
  - `pnpm build` passes. It fails on dead links.
  - Start `pnpm preview` and screenshot every new or changed page, in light and dark, with whatever browser tool the session has: Chrome DevTools MCP, Playwright, or `chromium --headless --screenshot`. Look at each screenshot.
  - Check that new pages appear in the sidebar.
  - Check that examples on other pages use the forms the reference pages define. A grep is enough.
- **Demo.** If commands or output changed, update `demo/bin/skl` and `demo/demo.tape`, re-render, and look at frames of the changed steps.
- **Code**, once it exists:
  - `go test ./...` passes.
  - Every newly documented behavior has a stub test in `spec/`, skipped until it's implemented.

Proofs stay out of git. Keep them, or a link to where `/testit` saved them, in `.workbench/<slug>/proofs/`. If `/testit` isn't available, do the same checks by hand and save the same proofs.

## 5. PR

1. Commit with a message that lists the changes and ends with `Closes #<issue>`.
2. Push the branch and open the PR. Its body holds:
   - `Closes #<issue>`;
   - the changes;
   - how they were verified, as a proof summary;
   - the follow-ups.
3. Add a `Delivery` section to the investigation README with the card, the branch and the PR link.

## 6. Hand over

Report the PR link and stop. Merging is the user's decision; once they approve, it's skl-merge's job.
