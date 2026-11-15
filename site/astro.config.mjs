import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';

// Published by GitHub Pages at the custom domain (public/CNAME), so the base
// is the root.
export default defineConfig({
  site: 'https://sona.coredev.uk',
  base: '/',
  integrations: [
    starlight({
      title: 'Sona',
      description: 'Sona is a native Apple Music client for Linux, macOS and Windows.',
      logo: { src: './src/assets/sona-mark.svg', alt: '' },
      favicon: '/favicon.svg',
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/coredev-uk/sona-go' }],
      editLink: { baseUrl: 'https://github.com/coredev-uk/sona-go/edit/main/site/' },
      customCss: ['./src/styles/sona.css'],
      plugins: [starlightLinksValidator()],
      sidebar: [
        {
          label: 'Guide',
          items: [
            { slug: 'guide/install' },
            { slug: 'guide/first-run' },
            { slug: 'guide/library' },
            { slug: 'guide/playback' },
            { slug: 'guide/settings' },
            { slug: 'guide/keyboard-shortcuts' },
            { slug: 'guide/scrobbling-and-discord' },
            { slug: 'guide/private-listening' },
            { slug: 'guide/themes' },
            { slug: 'guide/cli' },
            { slug: 'guide/troubleshooting' },
          ],
        },
        { slug: 'accessibility' },
        { slug: 'privacy-and-security' },
        {
          label: 'Contributing',
          items: [
            { slug: 'contributing/setup' },
            { slug: 'contributing/build-and-run' },
            { slug: 'contributing/test' },
            { slug: 'contributing/architecture' },
            { slug: 'contributing/ui-conventions' },
            { slug: 'contributing/add-a-feature' },
            { slug: 'contributing/translations' },
            { slug: 'contributing/pull-requests' },
            { slug: 'contributing/documentation' },
          ],
        },
        {
          label: 'Reference',
          items: [{ slug: 'reference/theme-format' }, { slug: 'reference/cli' }],
        },
      ],
    }),
  ],
});
