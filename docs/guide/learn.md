# Learn

::: warning Design preview
skl is not built yet. This page describes the planned behavior.
:::

Improve a skill you have with what another skill does better. Claude reads both, asks you what to take and how, and rewrites yours. You review the diff before anything lands. The skill you learn from is its **peer**.

```sh
skl learn someone/agent-skills/deep-pr-review review/pr-review \
  "I like its security checks"
```

The first argument is the peer, given as any [repo reference](../reference/repos) or as one of your own skills. The second is your skill. Leave it out to use your skill with the peer's name. The text at the end is optional and goes to Claude as is, so say what you want in your own words. Without it, Claude asks.

If you don't have the skill yet, the same command copies it: see [Get one](./other-skills#get-one).

## How a lesson works

1. **Analyse.** Claude maps what both skills share, what only the peer does, what only yours does, and where they contradict each other.
2. **Interview.** One question for each decision: what you asked for, and every contradiction. Your answers are saved.
3. **Rewrite.** Claude rewrites your skill in its own voice and structure, adding the new capability. The peer's scripts come along only if you said yes.
4. **Review.** skl shows the diff. You can ask for changes in plain words ("make the security part shorter").
5. **Land.** You replace your skill, save the result under a new name, or discard it.

```
→ lesson into review/pr-review
◇ peer only  OWASP checklist, secret scanning ← what you asked for
◇ clash      peer comments inline; yours writes one summary
? Inline comments or one summary?
  › keep my summary, add a Security section
? Bring along the peer's secret-scanning script?
  › yes, as an optional step
◇ rewritten  SKILL.md +38 −5
? Land it   › replace   save as review/pr-review-sec   discard
✓ committed · generation 12
```

Landing creates a vault commit and a generation, so `skl rollback` undoes it. Run `skl sync` to push it.

To learn from more than one peer, run `skl learn` once for each. Every run is its own lesson, and you can roll each one back separately.

## Which skill it changes

| Second argument | Your skill |
|---|---|
| none | the one with the peer's name: `pdf` |
| `pdf-tools` | the one with that name, in whichever group has it |
| `docs/pdf` | exactly that one |
| `docs/` | the one with the peer's name in your `docs` group |

A name matches a skill's folder or the `name` in its `SKILL.md`. If two groups have it, skl stops and asks you to write `group/name`. If no skill has it, a new one goes into a group named after the peer's owner: `someone/agent-skills/pdf` becomes `someone/pdf`, and a repo on this machine uses its folder name. A skill at the root of a repo (`someone/pdf-skill//`) takes the `name` in its `SKILL.md`, else the repo's name.

The last argument counts as text when it contains a space. So a one-word request needs more words, or it's read as a skill name.

skl prints what it's about to do before it does it:

```
→ copy into someone/pdf
→ lesson into review/pr-review
→ new skill someone/docx, learned from someone/agent-skills/docx
```

It asks only before someone else's new skill or scripts land (see [Get one](./other-skills#get-one)), and when a lesson is ready to land. If it isn't what you meant, `skl rollback` undoes it.

## A new skill from part of another

With text and a skill you don't have, Claude builds a new skill from the peer with only what you asked for. It runs like a [lesson](#how-a-lesson-works), starting from an empty skill: Claude asks what to bring along with that part, and which scripts to take.

```sh
skl learn someone/agent-skills/docx "only the tables part"
```

## When the peer changes

Peers keep improving. `skl sync` checks them and `skl status` shows `update available`: see [Keep up with its author](./other-skills#keep-up-with-its-author). To pick up what's new, give `learn` only your skill:

```sh
skl learn review/pr-review "only the new security checks"
```

skl uses the peer you learned from last, or for a copy the skill it was copied from. Running the first command again does the same. If the skill is a copy you never changed, skl takes the author's version as it is, without Claude. Otherwise it runs a lesson. Claude sees three versions: the peer as you last learned from it, the peer now, and your skill, plus skl's own attempt at merging them. So it can tell the peer's changes from yours, and asks only about what's new and what contradicts. It reuses your saved answers, and changes you skip aren't asked about again, unless your text asks for them.

Without text, Claude takes what's new. With text, it does what you say, even if the peer hasn't changed: "drop my changes, take theirs" brings back the author's version.

## Lessons

Each run with Claude is recorded in the skill's `.skl-origin.toml` as a lesson. It holds the peer, its commit, what you asked for, your answers and what you declined. A copy records its source instead. See [Files](../reference/files#skl-origin-toml).

## Who does the thinking

For a lesson, skl prepares a workspace with both skills and runs the `claude` CLI in it, using your existing login. Claude writes its result to a staging folder. Only skl commits to your vault, and only after you choose. A copy, and an update to a copy you never changed, need no Claude. Building a new skill from text, and updating a skill you changed, do: without the `claude` CLI, skl stops and says so.
