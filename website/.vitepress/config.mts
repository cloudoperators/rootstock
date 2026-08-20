import { withMermaid } from 'vitepress-plugin-mermaid'
import markdownItFootnote from 'markdown-it-footnote'

const docsVersion = process.env.DOCS_VERSION
const pagesBase = process.env.PAGES_BASE
const base = docsVersion && pagesBase
  ? `/${pagesBase}/${docsVersion}/`
  : docsVersion
    ? `/${docsVersion}/`
    : pagesBase
      ? `/${pagesBase}/`
      : '/'

export default withMermaid({
  title: 'Rootstock',
  description: 'Open source collector that scrapes telemetry from data center hardware and delivers it upstream for dashboards and carbon accounting.',

  base,
  cleanUrls: true,
  lastUpdated: true,

  head: [
    ['link', { rel: 'icon', href: `${base}favicon.svg`, type: 'image/svg+xml' }],
  ],

  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'Rootstock',

    nav: [
      { text: 'Getting Started', link: '/getting-started' },
      { text: 'Concepts', link: '/concepts/architecture' },
      { text: 'Reference', link: '/reference/api' },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Overview', link: '/getting-started' },
        ],
      },
      {
        text: 'Concepts',
        collapsed: false,
        items: [
          { text: 'Architecture', link: '/concepts/architecture' },
          { text: 'Design', link: '/concepts/design' },
        ],
      },
      {
        text: 'Reference',
        collapsed: false,
        items: [
          { text: 'API', link: '/reference/api' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/cloudoperators/rootstock' },
    ],

    search: {
      provider: 'local',
    },

    editLink: {
      pattern: 'https://github.com/cloudoperators/rootstock/edit/main/website/:path',
      text: 'Edit this page on GitHub',
    },

    outline: [2, 3, 4, 5],
  },

  markdown: {
    config: (md) => {
      md.use(markdownItFootnote)
    },
  },
})
