# Security policy

Report a vulnerability privately to the project maintainer before public
disclosure. Do not include sensitive source code or secrets in a report.

Blunt Code is intended to bind only to loopback and to execute analyzers using
direct process arguments. Loopback binding is not treated as an identity
boundary: each launch mints a random API token (stored only in the user's
`server.token`, mode 0600) that every API route except `GET /api/v1/health`
requires, either as `Authorization: Bearer` or as the HttpOnly, SameSite=Strict
session cookie the UI obtains from its sign-in link. Browser requests must come
from the exact app origin (scheme, host and port) and mutations accept only
JSON bodies, so other localhost pages cannot drive the API. Supported-version policy is established at first
public release.
