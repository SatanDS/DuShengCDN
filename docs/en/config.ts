import { defineAdditionalConfig, type DefaultTheme } from 'vitepress'

export default defineAdditionalConfig({
  description:
    'DuShengCDN is a lightweight, self-hosted OpenResty control plane for reverse proxy rules, releases, node sync, TLS certificates, and basic observability.',

  themeConfig: {
    nav: nav(),

    sidebar: {
      '/en/guide/': { base: '/en/guide/', items: sidebarGuide() },
      '/en/reference/': { base: '/en/reference/', items: sidebarReference() },
      '/en/design/': { base: '/en/design/', items: sidebarDesign() }
    },

    editLink: {
      pattern: 'https://github.com/SatanDS/DuShengCDN/edit/main/docs/:path',
      text: 'Edit this page on GitHub'
    },

    footer: {
      message: 'Released under the Apache License 2.0.',
      copyright: 'Copyright © DuShengCDN contributors'
    }
  }
})

function nav(): DefaultTheme.NavItem[] {
  return [
    { text: 'Guide', link: '/en/guide/', activeMatch: '/en/guide/' },
    { text: 'Reference', link: '/en/reference/', activeMatch: '/en/reference/' },
    { text: 'Design', link: '/en/design/', activeMatch: '/en/design/' }
  ]
}

function sidebarGuide(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: 'Guide',
      items: [
        { text: 'Overview', link: '' },
        { text: 'Quick Start', link: 'quick-start' },
        { text: 'Usage', link: 'usage' },
        { text: 'Deployment', link: 'deployment' },
        { text: 'SSO Login', link: 'sso' },
        { text: 'Run Server', link: 'server' },
        { text: 'Connect Agent', link: 'agent' },
        { text: 'Publish First Site', link: 'first-site' },
        { text: 'Upgrade and Maintenance', link: 'upgrade' },
        { text: 'Local Development', link: 'development' },
        { text: 'Troubleshooting', link: 'troubleshooting' }
      ]
    }
  ]
}

function sidebarReference(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: 'Reference',
      items: [
        { text: 'Overview', link: '' },
        { text: 'Configuration', link: 'configuration' },
        { text: 'Commands and Scripts', link: 'cli' },
        { text: 'API Conventions', link: 'api' },
        { text: 'Repository Layout', link: 'repository' }
      ]
    }
  ]
}

function sidebarDesign(): DefaultTheme.SidebarItem[] {
  return [
    {
      text: 'Design',
      items: [
        { text: 'Product Boundary', link: '' },
        { text: 'Architecture', link: 'architecture' },
        { text: 'Release Model', link: 'release-model' },
        { text: 'Development Constraints', link: 'development' }
      ]
    }
  ]
}
