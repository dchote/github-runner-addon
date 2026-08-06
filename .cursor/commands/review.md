# Code Review Command
---
description: Thorough code review to find issues and propose fixes
---

## Task

Review **$ARGUMENTS** thoroughly. Trace through all logic paths until you have the complete picture. Find:

- **Validate project goals and technical requirements** - review requirements as defined in `/docs/product-overview.md` and `/docs/technical-overview.md`
- **Dead/unused code** - functions, imports, variables, files with no callers
- **Dangling references** - broken imports, missing deps, orphaned calls after refactors
- **Stale/outdated content** - docs, comments, naming that doesn't match current code
- **Redundancy** - DRY violations, duplicate logic that should be consolidated
- **Over-complexity** - anything that could be simplified while maintaining functionality
- **Syntax errors** - attempt to build the code and run linters against it, automatically fix any build & linter issues found

For each finding, note: what, where (file:line), and why it's an issue.

## Output

Present a concise categorized summary of all issues found. Group by category, not by file.

Surface any questions or ambiguities that need clarification before fixes.

Do not implement fixes yet - just your helpful findings for review and approval, thanks!