# Security policy

## Supported versions

Blunt Code is pre-1.0 and ships from a single release line. Security fixes go
into the next release; older releases are not patched.

| Version | Supported |
| :--- | :--- |
| Latest [GitHub release](https://github.com/sanketpatel32/Blunt-code/releases/latest) | Yes |
| Any earlier release | No — update from the About page or rerun the installer |

## Reporting a vulnerability

Report vulnerabilities privately through GitHub's
[private vulnerability reporting](https://github.com/sanketpatel32/Blunt-code/security/advisories/new)
for this repository. Do not open a public issue, pull request or discussion for
an unfixed vulnerability.

Please include:

- the Blunt Code version (`bluntcode --version`) and Windows version;
- the affected area (CLI, loopback API, web UI, installer/updater, a managed
  tool install, a specific analyzer);
- steps to reproduce, and the impact you expect.

Do not include real secrets or proprietary source code. A minimal synthetic
repository that reproduces the issue is enough.

The maintainer will acknowledge the report, keep you updated in the advisory,
and credit you in the advisory and `CHANGELOG.md` unless you ask not to be
named. This is a one-maintainer project, so response times are best effort.

## Scope

In scope: vulnerabilities in code in this repository, including

- the loopback server being reachable from other machines, or its API being
  usable cross-origin from a web page;
- command or argument injection into analyzer processes;
- path traversal or zip-slip in workspace access, exports or managed-tool
  installs;
- bypass of SHA-256 verification for managed tools, the installer or the
  updater;
- source code, findings, or credentials (such as the managed SonarQube token)
  leaving the machine or appearing in logs and reports.

Out of scope: vulnerabilities in the third-party analyzers themselves (Ruff,
Biome, Semgrep, SonarQube, Gitleaks, OSV-Scanner, Trivy, Checkov, uv, Java).
Report those upstream; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)
for each project. Do report it here if Blunt Code pins a version with a known
vulnerability and a fixed release exists.

## Design commitments

Blunt Code binds only to `127.0.0.1`, runs analyzers as direct processes with
separate arguments (never through a shell), and makes only the outbound
requests listed in [docs/privacy.md](docs/privacy.md).
