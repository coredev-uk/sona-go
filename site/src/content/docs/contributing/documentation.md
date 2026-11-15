---
title: Write Docs
description: Where the documentation lives, how to preview it, and the rules for writing pages.
---

The public docs are the pages in `site/src/content/docs/`. The contributor and agent references (`docs/architecture.md`, `docs/design.md`, `docs/ui-architecture.md` and others) stay in the repository's `docs/` folder.

## Preview the site

```sh
devenv shell -- docs
```

This installs the dependencies and starts the site on `http://localhost:4321`. Pages reload when you save.

## Check before you push

```sh
cd site
npm run lint
npm run build
```

`npm run lint` checks the markdown. `npm run build` fails on a broken internal link. The Pages workflow runs both on every pull request that touches `site/`.

## Writing rules

- Lead with what the reader wants to do. Use second person, active voice and short sentences.
- Give each page one title in title case, and sentence-case headings that name the task: "Install on Linux".
- Open with one or two sentences, then the steps. Use a numbered list for steps, bullets for options and a table for reference facts. Nest lists at most two levels.
- Prefer a code block or a table to a paragraph.
- Keep history, internal reasons and implementation detail out of user pages. Link to `docs/` for depth.
- Use an aside only for a real warning or tip.
- Reference pages open with a one-line `>` description. Each command or token is a heading, its arguments are a bullet list (`name` type (optional) - description), and platform-only items carry _Linux_, _macOS_ or _Windows_.
