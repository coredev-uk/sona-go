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
      description: 'Sona is an Apple Music client for the desktop, built first for Linux.',
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
            { slug: 'guide/themes' },
            { slug: 'guide/cli' },
          ],
        },
        {
          label: 'Contributing',
          items: [{ slug: 'contributing/documentation' }],
        },
        {
          label: 'Reference',
          items: [{ slug: 'reference/theme-format' }, { slug: 'reference/cli' }],
        },
      ],
    }),
  ],
});
