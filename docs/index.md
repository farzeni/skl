---
layout: home

hero:
  name: skl
  text: Your Claude Code skills, in one repo, on every machine.
  tagline: skl keeps your skills in a git repo you own, loads only the ones you need, and lets you undo any change. It also brings in skills from other people's repos, and can teach yours what theirs do better.
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Why skl
      link: '#why-skl'

features:
  - title: Same skills everywhere
    details: Run skl init you/skills @backend on a new machine and your skills are there. skl sync keeps machines in step and merges edits made on both.
  - title: Edit freely, undo anything
    details: Keep improving skills in Claude Code. skl saves every change to your vault, and skl rollback takes any of them back.
  - title: Get any skill, or learn part of it
    details: skl learn copies a skill from any git repo into your vault, or lets Claude add what it does better to a skill you have. The same command takes its author's later changes.
  - title: Only what you need
    details: Load a group, one skill or a saved mix like @backend, per machine or per repo. Nothing is deleted to make room.
---

<img class="demo" src="/demo.gif" alt="Terminal recording: skl init, skl learn, skl status, a lesson, skl history, skl rollback">

::: warning Design preview
skl is not built yet. These docs describe the planned behavior, and the recording above runs a scripted mock.
:::

## Why skl

Claude Code reads your skills from `~/.claude/skills` on each machine. That folder isn't versioned and isn't shared, so a skill you improve on your laptop stays there, and a change you regret can't be taken back. Claude Code also lists every skill in it to Claude on each turn. Past a budget it drops descriptions, and Claude picks those skills less often ([Claude Code docs](https://code.claude.com/docs/en/skills#skill-descriptions-are-cut-short)).

skl keeps your skills in a git repo you own, copies only the ones you choose into that folder, and saves your edits back. Every change is a step you can undo.

**When you don't need skl:** if you use a few skills on one machine, the folder is enough. If you only install other people's skills and never change them, Claude Code plugins or `npx skills` do that.
