# QA Workflow

## Collaboration model

This project is intentionally run with a lightweight customer-developer workflow.

Roles:

- An AI coding assistant is the primary developer and technical owner of implementation details.
- The user acts as customer, product reviewer, QA tester, and owns all git operations (commits, branches, pushes).

## How work should proceed

1. A small iteration is selected from the plan.
2. The assistant implements the agreed scope (for larger changes: a written plan first, approved before code).
3. The assistant verifies the result locally as far as possible (build, tests, typecheck).
4. The user reviews the working tree, tests behavior, and commits when satisfied.
5. The user reports:
   - bugs
   - UX issues
   - missing cases
   - enhancement requests
   - changed business expectations
6. The assistant incorporates feedback and continues.

## What QA feedback should focus on

- whether the feature matches the business need
- whether the workflow feels correct
- whether the UI is understandable
- whether permissions behave as expected
- whether important edge cases are missing

## Preferred feedback format

Useful QA feedback examples:

- "The detail page is confusing for portal entries."
- "A support user can see a secret they should not be able to reveal."
- "The Key Vault list needs filtering by vault and expiry."
- "The SSH flow should prefer username and host together in one action."

## Change handling

Not every iteration needs a perfect plan upfront. Product understanding can evolve during QA. When direction changes:

- the relevant docs should be updated
- the next iteration should be re-scoped if needed
- code should follow the latest confirmed plan

## Release style

- small increments
- working software at each step
- the user commits after review
- QA before broadening scope

## Local runtime expectations

Docker Desktop may be used to validate complete local runs when needed. This is especially useful once integration and multi-container behavior become more important.
