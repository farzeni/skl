# Third party

::: warning Design preview
Nothing on this page is built yet. See [What works in v0.1](/design/roadmap#what-works-in-v0-1).
:::

Core skl needs only git, and an agent's CLI for lessons: `claude`, or any [engine](../guide/learn#other-agents) you set. The parts on these pages work with services and tools other people run. Each one is optional and kept apart from the rest of skl: if one is removed, or the service behind it goes away, every other command works as before.

| Part | Adds |
|---|---|
| [skills.sh](./skills-sh) | `skl search`, and skills.sh links as repo references |
| [npx skills](./npx-skills) | `skl import` remembers where a skill installed by `npx skills` came from |
