package selfupdate

// ReleaseSigningKey is the base64 Ed25519 public key that release checksums
// must be signed with (see cmd/releasesign and scripts/package.ps1). When it
// is set, an update without a valid .sha256.sig asset is refused. Generate
// the pair with `go run ./cmd/releasesign keygen`, keep the private key out
// of the repository, and paste the public key here (or inject it with
// -ldflags "-X bluntcode/internal/selfupdate.ReleaseSigningKey=...").
var ReleaseSigningKey = ""
