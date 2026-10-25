---
name: skl-implement
description: >-
  Step 2 of the skl workflow (skl-investigate → skl-implement → skl-merge). Turn a closed
  .workbench investigation (docs mode) or a code card from the SKL Board (code mode) into a
  change: work in a worktree, apply the plan or the card, verify it with /testit, and open a PR
  that closes the card. In code mode, write the spec scripts first, run make check, then iterate
  with a separate validator (skl-validate) until it says GREEN. Use when the user or a
  coordinator says to apply, implement, proceed with or "do the real change" in the skl project,
  or hands you a card number.
argument-hint: "<investigation slug | card number>"
---

# skl-implement

Step 2 of 3. It starts from a **closed investigation** left by [skl-investigate](../skl-investigate/SKILL.md), or from a **code card**. It ends with an **open PR** for [skl-merge](../skl-merge/SKILL.md). The board ids and the commands for setting a card's status are in `CLAUDE.md` under "Board".

Input: `$ARGUMENTS`. It picks one of two modes:

- **Docs mode:** the slug of a folder in `.workbench/`. The investigation is the plan.
- **Code mode:** an issue number for a **code card**. A code card is an issue with a `sprint-N` label whose body has `Docs implemented` and `Acceptance` sections. The card and the doc pages it names replace the investigation, so no `.workbench/` folder is needed.

In code mode you are the **implementer**. A separate **validator** checks your PR with [skl-validate](../skl-validate/SKILL.md). The **coordinator** is the session that started you (in the build, SKL). Without one, the user is the coordinator.

## 0. Check the state

- **Docs mode: the investigation has `Status: closed`.** If it's still open, say what's missing (open questions, untested points) and go back to skl-investigate. The only exception is a request the user fully specified and chose not to investigate. Then the request is the plan.
- **Code mode: the card can start.**
  - It is in **Todo**, or already **In Progress** if you are resuming it.
  - Every `Depends on #N` issue is closed. If one isn't, tell the coordinator and stop.
- **Code mode: read before you write.** Read `CLAUDE.md`, `CONTRIBUTING.md` (architecture, errors, context, output, testing layers; before it exists, wherever the card says), the card, and every doc page and section the card names, all the way through.

## 1. Card

- **Docs mode.** `.workbench/` isn't in git, so the issue must stand on its own. Its body holds the question, the findings with key evidence, the decisions and the plan.

  ```sh
  gh issue create --repo farzeni/skl --title "<what will change>" --body-file <file>
  ```

  The board adds new issues on its own.
- **Code mode.** The card already exists. Skip creating it.

Either way, find the card's item id with `gh project item-list` and set it to **In Progress**.

## 2. Worktree

```sh
git fetch -q && git worktree add .claude/worktrees/<issue>-<slug> -b <issue>-<slug> origin/main
cd .claude/worktrees/<issue>-<slug>
(cd docs && pnpm install --frozen-lockfile)   # only if the change touches docs/
```

All edits and all git commands happen in the worktree. Never on `main`, and never in the shared checkout. To pick up newer work, merge `origin/main` into the branch. Never rebase a pushed branch or force-push.

## 3. Apply the plan or the card, and only that

- Follow the writing rules in `CLAUDE.md`, and in code mode `CONTRIBUTING.md`.
- **Code mode: spec tests first.** For each doc sentence under the card's `Docs implemented`, write the testscript block that checks it, preceded by `# "<doc sentence>"` (see "Stub tests" in `CLAUDE.md`). A package that no command uses yet checks its sentences with tests in the package instead (table or golden), each naming its sentence. Watch the tests fail, then write the code until they pass. Use `skip 'pending: <page>'` only for a sentence the card leaves for later, and list it in the PR.
- Follow the card's `Design`. Stay inside its `Packages and files touched`, plus `spec/testdata/` for the card's own sentences. Anything wrong you notice outside them goes on a follow-up list for the PR. Don't fix it here.
- **If the code shows a doc is wrong or silent, stop.** Tell the coordinator, giving the page, the sentence and the evidence. A docs PR comes first, through skl-investigate. A change to a doc's meaning or to a `CLAUDE.md` decision needs the user.
- Take easy, reversible calls yourself and list them in the PR. Ask the coordinator about anything the card, `CONTRIBUTING.md` and the docs don't settle.
- If the plan turns out to be wrong in docs mode, stop and go back to skl-investigate.

## 4. Verify with /testit

Run `/testit` on the branch. Its claims are the plan items and decisions (docs mode), or the card's acceptance criteria and doc sentences (code mode). For this project they mean:

- **Code**
  - `make check` passes. CI runs exactly `make check`: format, lint, `go test -race ./...` (spec scripts included), govulncheck and `goreleaser check`.
  - A card that changes no Go code (workflow skills, say) has nothing for `make check` to cover. Read each changed file as the agent that will follow it, and dry-run a sample card instead.
  - Every doc sentence under `Docs implemented` has a passing script block, or package test, that checks what the sentence says, not just its comment.
  - Run the built `skl` against a temporary `HOME` for each acceptance criterion and look at the output: marks, wording, exit code. Never touch the real `~/.claude` or `~/.skl`.
- **Docs**, only when `docs/` changed
  - `pnpm build` passes. It fails on dead links.
  - Start `pnpm preview` and screenshot every new or changed page, in light and dark, with whatever browser tool the session has: Chrome DevTools MCP, Playwright, or `chromium --headless --screenshot`. Look at each screenshot.
  - Check that new pages appear in the sidebar.
  - Check that examples on other pages use the forms the reference pages define. A grep is enough.
- **Demo.** If commands or output changed, update `demo/bin/skl` and `demo/demo.tape`, re-render, and look at frames of the changed steps.

Proofs stay out of git, in the main checkout's `.workbench/`: `.workbench/<slug>/proofs/` in docs mode, `.workbench/YYYY-MM-DD-card-<issue>/proofs/` in code mode (the card's folder, dated the day work started; reuse it if it exists). If `/testit` isn't available, do the same checks by hand and save the same proofs.

## 5. PR

1. Commit with a message that lists the changes and ends with `Closes #<issue>`.
2. Push the branch and open the PR. Its body holds:
   - `Closes #<issue>`;
   - **code mode:** each doc sentence it implements, with its page and the script that checks it, and any sentence left `skip`;
   - the changes;
   - the calls you took;
   - how it was verified, as a proof summary;
   - the follow-ups.
3. **Docs mode:** add a `Delivery` section to the investigation README with the card, the branch and the PR link.

## 6. Hand over

- **Docs mode.** Report the PR link and stop. Merging is the user's decision. Once they approve, it's skl-merge's job.
- **Code mode.** Get the PR validated, then merge it:
  1. Ask for a validator. You never validate your own PR. In a swarm, message the coordinator `ready for validation · PR #<n>`. Otherwise, ask the user to start a new session with `/skl-validate <n>`.
  2. The validator answers **GREEN** or numbered findings, as a message and as a PR comment headed `skl-validate · round <k>`. You never post a comment with that header or with `GREEN at`.
  3. For each finding, fix it or disagree with evidence. Push new commits, then reply to the validator, where its findings came from. Say what changed for each finding and give the new head sha.
  4. After 3 rounds without GREEN, stop and tell the coordinator.
  5. On GREEN for the current head, run skl-merge. The validator's GREEN replaces the user's "merge". A push after GREEN voids it.
