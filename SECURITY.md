# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities **privately**. Do not open a public
issue for a suspected vulnerability.

- Preferred: use GitHub's
  [private vulnerability reporting](https://github.com/hk9890/task-manager-ui/security/advisories/new)
  ("Report a vulnerability" under the repository's **Security** tab).
- Alternatively, email the maintainer at **hans.kohlreiter@gmail.com** with
  details and, if possible, a minimal reproduction.

Please include:

- A description of the issue and its impact.
- Steps to reproduce (commands, inputs, environment).
- The affected version or commit.

You can expect an initial acknowledgement within a few days. Once a fix is
available, a patched release will be published and the reporter credited unless
anonymity is requested.

## Supported versions

This is an early-stage project. Security fixes are applied to the latest
released version on the `main` branch. Older tagged releases are not
back-patched.

## Scope

`taskmgr-ui` is a local terminal application that reads and writes a file-based
`.tasks/` store and can launch external tools configured by the user. Of
particular relevance:

- **Launcher commands** run external processes using user-provided
  configuration. Treat your config file as trusted input.
- The app makes no network calls of its own at runtime.
