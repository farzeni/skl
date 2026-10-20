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
      { text: 'Design', link: '/design/architecture' }
    ],
    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting started', link: '/guide/getting-started' },
          { text: 'Concepts', link: '/guide/concepts' },
          { text: 'Loading skills', link: '/guide/loading' },
          { text: 'Rollback', link: '/guide/rollback' },
          { text: 'Fork', link: '/guide/fork' },
          { text: 'Learn', link: '/guide/learn' }
        ]
      },
      {
        text: 'Reference',
        items: [
          { text: 'Commands', link: '/reference/commands' },
          { text: 'Files', link: '/reference/files' }
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
