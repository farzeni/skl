import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'skl',
  description: 'Your Claude Code skills, on any machine.',
  cleanUrls: true,
  lastUpdated: true,
  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/getting-started' },
      { text: 'Reference', link: '/reference/commands' },
      { text: 'Third party', link: '/thirdparty/' },
      { text: 'Design', link: '/design/architecture' }
    ],
    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting started', link: '/guide/getting-started' },
          { text: 'Concepts', link: '/guide/concepts' },
          { text: 'Your skills', link: '/guide/skills' },
          { text: 'Skill sets', link: '/guide/skill-sets' },
          { text: 'Sync', link: '/guide/sync' },
          { text: 'Rollback', link: '/guide/rollback' },
          { text: 'Fork', link: '/guide/fork' },
          { text: 'Learn', link: '/guide/learn' }
        ]
      },
      {
        text: 'Reference',
        items: [
          { text: 'Commands', link: '/reference/commands' },
          { text: 'Repo references', link: '/reference/repos' },
          { text: 'Configuration', link: '/reference/config' },
          { text: 'Files', link: '/reference/files' }
        ]
      },
      {
        text: 'Third party',
        items: [
          { text: 'Overview', link: '/thirdparty/' },
          { text: 'skills.sh', link: '/thirdparty/skills-sh' },
          { text: 'npx skills', link: '/thirdparty/npx-skills' }
        ]
      },
      {
        text: 'Design',
        items: [
          { text: 'Architecture', link: '/design/architecture' },
          { text: 'Roadmap', link: '/design/roadmap' }
        ]
      }
    ],
    search: { provider: 'local' },
    outline: 'deep'
  }
})
