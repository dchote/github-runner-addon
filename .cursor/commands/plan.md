# Plan Command
---
description: Think through completely, then present concise plan
---

## Task

Based on the research context we've built, think through **$ARGUMENTS** completely.

1. **Work through it fully** - Consider all impacts, dependencies, edge cases. Be exhaustive and thorough. Make sure you have the complete picture before summarizing.

2. **Review project goals and technical requirements** as defined in `/docs/product-overview.md` and `/docs/technical-overview.md`

3. **Identify all changes needed** - What files, functions, references, docs need updating?

4. **Order by dependencies** - What needs to happen first? Group related changes.

## Output

Once you're sure you have it all worked out, present a concise but complete action plan:

- Categorized list of changes (no code snippets)
- Clear sequence if order matters
- Any risks or open questions

Plan only - no implementation yet. I'll review and refine before we proceed. Thanks, I appreciate you!

Write the plan into `docs/features/<N>-<feature-name>.md` using the next available feature number (starting with 0001) and a kebab-case feature name derived from the task. Example: `0001-bar-data-import.md`