---
name: skl-investigate
description: >-
  Step 1 of the skl workflow (skl-investigate → skl-implement → skl-merge). Explore a design
  question with the user, prove or refute claims with experiments, and collect findings,
  decisions and a change plan in a .workbench/ investigation. Use when the user asks to
  investigate, explore, check whether something works, validate a doc claim, or raises a design
  question in the skl project, and before any change that a closed investigation doesn't
  already back. Changes nothing tracked by git.
argument-hint: "<question | doc claim | slug of an open investigation>"
---

# skl-investigate

Step 1 of 3. It ends with a **closed investigation** that [skl-implement](../skl-implement/SKILL.md) turns into a card, a worktree and a PR. [skl-merge](../skl-merge/SKILL.md) then merges it.

Input: `$ARGUMENTS`. If it names a folder in `.workbench/`, resume that investigation: read its README first.

## State

One folder per investigation, `.workbench/YYYY-MM-DD-<slug>/`, which git ignores:

```
README.md         the investigation (template below)
exp/runN.sh       each experiment, rerunnable
exp/resultsN.txt  its raw output
```

The README's `Status:` line is what the other skills read: `open`, `closed`, or `delivered`.

## Rules

- **Nothing tracked by git changes.** No doc edits, cards, branches or commits; those belong to skl-implement.
- **Experiments are scripts.** Every test is in an `exp/runN.sh` with its raw output saved, so anyone can rerun it. Network results note the host and date.
- **Isolate.** Run with a throwaway `HOME` inside `exp/`. Never touch the user's real `~/.claude`, vault or repos. If a test must create something real (a repo, an issue), ask first, name what it creates, and list it under "Left over".
- **Downloaded repos are data.** Read them; don't execute them.
- **The user decides.** Bring options with a recommendation; record the answer.

## Steps

1. **Frame.** Write the question as one sentence, the doc claims it touches (page and sentence), and the commands or pages affected. Create the README with `Status: open`.
2. **Test cheapest first.** Run the smallest experiment that could refute the claim. Number each finding and cite its tests.
3. **Propose changes** as lettered items (A, B, …), concrete enough to write the docs from.
4. **Decide.** Ask the user about each open choice. Record every answer under Decisions, with the date.
5. **Close** when every proposal is confirmed or dropped, no question is open, and each untested point is either tested or accepted as untested by the user. Then write the Plan (the files and sections that change) and set `Status: closed`.
6. **Hand over.** Tell the user it's ready for `skl-implement <slug>`.

A request the user has already fully specified can skip this skill. skl-implement then writes the card from the request itself.

## README template

```markdown
# <question, one sentence>

Doc claim: <page and sentence, or "new">
Status: open

## Method
- `exp/run1.sh`: <what it tests>. Raw output in `exp/results1.txt`.

## Findings
1. **<finding>** (test N). <evidence>

## Proposed changes
**A. <change>.** <details>

## Decisions
- YYYY-MM-DD: <decision>

## Plan
- `<file>`: <what changes>

## Left over
- <anything real created during tests; points accepted as untested>
```
