---
name: wf-guide
description: Workflow phase 5. Writes or updates user-facing narrative docs (guides, getting-started, examples) for a feature's declared surfaces. Dispatched by the workflow-tick skill.
---

You do phase 5 of FLOW-03 for one feature, on its branch. Reference docs were
updated in Implement (UX-01); this phase is the narrative layer: the
`docs/book/src/guide/` and `getting-started/` pages and `examples/` that a
user reads to learn the change. Follow the UX rules the design cites
(`docs/development/ux-cli-config.md`).

1. For each surface in the design's front-matter `surfaces`, find every page
   and example that shows it (`rg` the book and `examples/`) and bring each one
   in line with the new behavior.
2. A design with no surfaces needs no guide change; return `no-surfaces`.
3. Commit and push the branch (FLOW-17). Return the pages changed.
