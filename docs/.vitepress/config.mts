import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'skl',
  description: 'Your agent skills, on any machine.',
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
          { text: 'Your skills', link: '/guide/skills' },
          { text: 'Rollback', link: '/guide/rollback' },
          { text: 'Skill sets', link: '/guide/skill-sets' },
          { text: 'Learn', link: '/guide/learn' },
          { text: 'Sync', link: '/guide/sync' }
        ]
      },
      {
        text: 'Reference',
        items: [
          { text: 'Commands', link: '/reference/commands' },
          { text: 'Selectors and references', link: '/reference/repos' },
          { text: 'Configuration', link: '/reference/config' },
          { text: 'Files', link: '/reference/files' },
          { text: 'Glossary', link: '/reference/glossary' }
        ]
      },
      {
        text: 'Third party',
        items: [
          { text: 'Overview', link: '/thirdparty/' },
          { text: 'skills.sh', link: '/thirdparty/skills-sh' },
          { text: 'npx skills', link: '/thirdparty/npx-skills' },
          { text: 'opencode', link: '/thirdparty/opencode' },
          { text: 'pi', link: '/thirdparty/pi' },
          { text: 'gh', link: '/thirdparty/gh' }
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
