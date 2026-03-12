import { defineConfig } from 'vitepress'
import { execSync } from 'child_process'

const commitHash = execSync('git rev-parse --short HEAD').toString().trim()

export default defineConfig({
  title: 'willow',
  description: 'A git worktree manager built for AI agent workflows.',
  appearance: 'force-dark',
  cleanUrls: true,
  sitemap: { hostname: 'https://getwillow.dev' },

  head: [
    ['link', { rel: 'icon', href: '/favicon.svg', type: 'image/svg+xml' }],
    ['meta', { name: 'theme-color', content: '#131318' }],
    ['meta', { property: 'og:title', content: 'willow — Git worktree manager for AI agents' }],
    ['meta', { property: 'og:description', content: 'Spin up isolated worktrees for Claude Code sessions. Switch between them instantly with fzf. See which agents are busy, waiting, or idle.' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:url', content: 'https://getwillow.dev' }],
    ['link', { rel: 'canonical', href: 'https://getwillow.dev' }],
  ],

  themeConfig: {
    logo: '/favicon.svg',
    siteTitle: 'willow',

    nav: [
      { text: 'Guide', link: '/guide/' },
      { text: 'Commands', link: '/commands/' },
      { text: 'Tmux', link: '/tmux/' },
      { text: 'Configuration', link: '/configuration/' },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Guide', link: '/guide/' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Commands', link: '/commands/' },
          { text: 'Tmux Integration', link: '/tmux/' },
          { text: 'Configuration', link: '/configuration/' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/iamrajjoshi/willow' },
    ],

    search: {
      provider: 'local',
    },

    footer: {
      message: `MIT | Built by <a href="https://github.com/iamrajjoshi" target="_blank">@iamrajjoshi</a>`,
      copyright: `<a href="https://github.com/iamrajjoshi/willow/commit/${commitHash}" target="_blank">${commitHash}</a>`,
    },
  },
})
