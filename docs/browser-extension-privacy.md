# Access Workspace Browser Fill — Privacy Policy

_Last updated: 2026-07-28_

Access Workspace Browser Fill is a companion extension for a self-hosted
Access Workspace deployment. It suggests and fills portal credentials that
your organization's workspace has approved for you, and can offer to save
credentials you enter back into that workspace.

## What the extension stores on your device

The extension keeps the following in the browser's local extension storage:

- the address of the Access Workspace deployment you connected it to
- a session token issued by that workspace when you connect the extension
- a random installation identifier (used so your workspace can list and
  revoke connected browsers)
- your preference for credential save prompts
- a pending "save this login?" candidate (site address, username, password)
  for at most 10 minutes after you submit a login form, only so the save
  prompt can be shown; it is discarded afterwards or when you dismiss it
- short-lived "don't suggest again on this site" dismissals (15 minutes)

Nothing is stored outside the browser profile, and disconnecting the
extension discards the session token.

## What the extension transmits, and to whom

The extension communicates **only with the Access Workspace deployment you
explicitly connected it to** (the address is configured at connect time and
shown in the popup). It sends:

- the address of the page you are on — normalized, with query string and
  fragment removed — to ask that workspace whether it holds an approved
  credential for that portal
- credentials you explicitly choose to save or update, when you accept the
  save prompt
- standard authenticated API calls (connect, session check, sign-out)

The extension sends nothing to the extension's authors, to any analytics
service, or to any other third party. There is no telemetry, no tracking,
and no advertising.

## What the extension reads on pages

To detect login forms and fill approved credentials, a content script runs
on pages you visit. It inspects form fields locally in the page; page
content is not transmitted anywhere except as described above (the page
address for credential matching).

## Credentials

Credential secrets are fetched from your workspace on demand when you ask
the extension to fill them, are handed to the login form, and are not
retained by the extension afterwards. Whether a credential may be filled is
decided by your workspace's per-object policy.

## Your organization

The workspace deployment the extension talks to is operated by your own
organization, not by the extension's authors. Your organization's own
policies govern the data stored there (including audit records of
credential use).

## Contact

Questions about this policy or the extension: open an issue on the
project repository, or contact the team operating your Access Workspace
deployment.
