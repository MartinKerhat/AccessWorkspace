# Security Policy

Access Workspace stores and brokers credentials, so security reports are taken
seriously and handled ahead of feature work.

## Reporting a vulnerability

**Do not open a public issue.** Report through
[GitHub's private vulnerability reporting](https://github.com/MartinKerhat/AccessWorkspace/security/advisories/new)
on this repository — Security → Report a vulnerability. It is private to the
maintainer and keeps the whole exchange in one place.

Useful reports include what an attacker can achieve, the steps to reproduce it,
and which component is involved (backend, frontend, launcher, browser
extension). A proof of concept helps but is not required — a clear description
of the flaw is enough to start.

You will get an acknowledgement within a few working days. Because this is a
small project maintained alongside other work, please allow reasonable time for
a fix before public disclosure.

## Scope

In scope: this repository — backend, frontend, desktop launcher, browser
extension, and the deployment assets here.

Out of scope: vulnerabilities in Azure, Entra, or Azure Key Vault themselves
(report those to Microsoft), and findings that depend on a deployment being
misconfigured against the documented guidance — for example running with a
shared `RESOURCE_SECRET_KEY`, exposing the database, or enabling demo seeding in
production. Weaknesses in that documented guidance itself *are* in scope.

## Known and accepted design limits

These are deliberate, documented in [docs/security.md](docs/security.md), and
not vulnerabilities:

- **A personal vault cannot be recovered** if the user loses every unlock
  method. Nothing can re-derive the private key; the mitigation is enrolling
  more than one method.
- **An admin-forced password reset destroys the user's personal secrets.** A
  reset that could recover them would mean an admin could read them.
- **A shared secret is readable by everyone it is shared with** when the object
  allows reveal or copy. Reveal and copy are one permission on purpose — anyone
  who can copy a secret can read it.
- **The launcher runs a loopback bridge** on `127.0.0.1:47654` and executes
  native clients on the user's machine, with launches gated behind one-time
  tickets issued by the workspace.

## Supported versions

This project ships from `main`; fixes land there. There are no maintained
release branches.
