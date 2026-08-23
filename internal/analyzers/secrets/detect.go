package secrets

// Pure detection engine for the built-in secrets analyzer. Everything in this
// file operates on byte slices held in memory and never touches the network,
// the filesystem, or an external process, so the detector is fully testable
// and safe to run inside the scan pipeline.
//
// Rule IDs are STABLE public identifiers: finding fingerprints hash them, so
// renaming a rule would orphan every stored finding. The scheme is
// secrets.<name> and the set is fixed unless a rule is retired deliberately.

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Scanning hygiene limits. Files larger than maxScanBytes are read only up to
// the cap, files containing a NUL byte in their first binarySniffBytes bytes
// are treated as binary and skipped, and detection results are capped per file
// and per run so a pathological input cannot flood a scan.
const (
	maxScanBytes       = 1 << 20 // 1 MiB
	binarySniffBytes   = 8 << 10 // 8 KiB
	maxFindingsPerFile = 50
	maxFindingsPerRun  = 5000
)

// Detection thresholds for the generic assignment rule.
const (
	// minEntropyBits is a simple Shannon-entropy floor over the character
	// distribution of an assigned literal. Plain words and repeated filler
	// ("administrator"-style values, "AAAAAAAA") fall below it while real
	// credential material clears it comfortably.
	minEntropyBits = 3.0
)

// Stable rule identifiers.
const (
	ruleAWSAccessKey    = "secrets.aws-access-key-id"
	ruleGitHubToken     = "secrets.github-token"
	ruleSlackToken      = "secrets.slack-token"
	ruleGoogleAPIKey    = "secrets.google-api-key"
	rulePrivateKeyBlock = "secrets.private-key-block"
	ruleJWT             = "secrets.jwt"
	ruleConnectionURI   = "secrets.connection-uri"
	ruleGenericAssign   = "secrets.generic-assignment"
)

// Go's regexp (RE2) has no lookaround, so word-boundary enforcement is done in
// code: a match only counts when the bytes around it are outside the rule's
// token alphabet. That keeps prose mentions ("the AKIA prefix marks AWS
// keys") and longer surrounding tokens from firing.
type byteSet string

func (s byteSet) has(b byte) bool { return strings.IndexByte(string(s), b) >= 0 }

const (
	setLettersDigits   byteSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	setAlnumUnderscore byteSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_"
	setTokenish        byteSet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
)

var (
	awsKeyRe     = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	githubRe     = regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`)
	slackRe      = regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)
	googleRe     = regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)
	privateKeyRe = regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |PGP )?PRIVATE KEY-----`)
	jwtRe        = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)
	// Group layout: 1 scheme, 2 user, 3 password. The password group must be
	// non-empty and is additionally screened against placeholder samples like
	// user:pass@example.com by validateURI.
	uriRe = regexp.MustCompile("(postgres(?:ql)?|mysql|redis|amqp|mongodb(?:\\+srv)?|https?)://([^\\s:@/\"']+):([^\\s@/\"']+)@[^\\s\"'<>]+")
	// Generic hardcoded-credential assignment. RE2 has no backreferences, so
	// the closing quote is matched as either quote character; a mismatched
	// pair is still an assignment of a quoted literal. Group 1 is the key,
	// group 2 the value. The value must be at least 8 characters and stay on
	// one line.
	genericRe = regexp.MustCompile(`(?i)["']?(auth_token|access_key|api_key|apikey|passwd|password|secret|token)["']?[ \t]*(?::=|=|:)[ \t]*["']([^"'\n]{8,})["']`)
)

type structuredRule struct {
	id       string
	kind     string
	re       *regexp.Regexp
	left     byteSet // bytes that invalidate a match when directly before it
	right    byteSet // bytes that invalidate a match when directly after it
	validate func(data []byte, loc []int) bool
}

var structuredRules = []structuredRule{
	{id: ruleAWSAccessKey, kind: "AWS access key ID", re: awsKeyRe, left: setAlnumUnderscore, right: setAlnumUnderscore},
	{id: ruleGitHubToken, kind: "GitHub token", re: githubRe, left: setAlnumUnderscore, right: setAlnumUnderscore},
	{id: ruleSlackToken, kind: "Slack token", re: slackRe, left: setTokenish, right: setTokenish},
	{id: ruleGoogleAPIKey, kind: "Google API key", re: googleRe, left: setAlnumUnderscore, right: setTokenish},
	{id: rulePrivateKeyBlock, kind: "private key block", re: privateKeyRe},
	{id: ruleJWT, kind: "JSON Web Token", re: jwtRe, left: setTokenish, right: setTokenish},
	{id: ruleConnectionURI, kind: "connection URI with embedded credentials", re: uriRe, left: setAlnumUnderscore, validate: validateURI},
}

// match is one raw detection. secret never leaves the detector unredacted:
// only its first characters and its length are copied into diagnostics.
type match struct {
	rule   string
	kind   string
	key    string // assignment key for the generic rule
	start  int    // byte offset of the secret itself
	end    int
	secret string
}

func boundaryOK(data []byte, start, end int, left, right byteSet) bool {
	if left != "" && start > 0 && left.has(data[start-1]) {
		return false
	}
	if right != "" && end < len(data) && right.has(data[end]) {
		return false
	}
	return true
}

// validateURI rejects documentation samples such as user:pass@example.com.
func validateURI(data []byte, loc []int) bool {
	user := strings.ToLower(string(data[loc[4]:loc[5]]))
	password := string(data[loc[6]:loc[7]])
	if isPlaceholderValue(password) {
		return false
	}
	switch user {
	case "user", "username", "youruser", "your-user", "foo":
		// user:<anything> with a short password is the classic sample shape;
		// a real credential behind a placeholder username is longer.
		return len(password) >= 12
	}
	return true
}

// detect runs every rule over one file's content. Structured rules run first;
// the generic assignment rule skips values that a structured rule already
// reported, so a committed JWT assigned to `token` yields one finding, not two.
func detect(data []byte) []match {
	var out []match
	for _, rule := range structuredRules {
		for _, loc := range rule.re.FindAllSubmatchIndex(data, -1) {
			start, end := loc[0], loc[1]
			if !boundaryOK(data, start, end, rule.left, rule.right) {
				continue
			}
			if rule.validate != nil && !rule.validate(data, loc) {
				continue
			}
			out = append(out, match{rule: rule.id, kind: rule.kind, start: start, end: end, secret: string(data[start:end])})
		}
	}
	for _, loc := range genericRe.FindAllSubmatchIndex(data, -1) {
		// The underscore is a legal part of compound keys (db_password), so
		// only letters and digits invalidate the left boundary here.
		if loc[0] > 0 && setLettersDigits.has(data[loc[0]-1]) {
			continue
		}
		valueStart, valueEnd := loc[4], loc[5]
		value := string(data[valueStart:valueEnd])
		if isPlaceholderValue(value) {
			continue
		}
		if shannonEntropy(value) < minEntropyBits {
			continue
		}
		if matchesStructured(value) {
			continue
		}
		out = append(out, match{
			rule:   ruleGenericAssign,
			kind:   "hardcoded credential",
			key:    strings.ToLower(string(data[loc[2]:loc[3]])),
			start:  valueStart,
			end:    valueEnd,
			secret: value,
		})
	}
	return out
}

// matchesStructured reports whether s itself satisfies a structured rule; the
// generic rule stands down in that case.
func matchesStructured(s string) bool {
	data := []byte(s)
	for _, rule := range structuredRules {
		loc := rule.re.FindSubmatchIndex(data)
		if loc == nil {
			continue
		}
		if !boundaryOK(data, loc[0], loc[1], rule.left, rule.right) {
			continue
		}
		if rule.validate != nil && !rule.validate(data, loc) {
			continue
		}
		return true
	}
	return false
}

// placeholderValues are literals that mean "fill me in", not credentials.
// Trailing digits and punctuation are stripped before lookup so "todo1234"
// and "changeme!" are caught too.
var placeholderValues = map[string]bool{
	"changeme": true, "change-me": true, "change_me": true,
	"example": true, "examples": true, "sample": true, "placeholder": true,
	"todo": true, "fixme": true, "test": true, "tests": true, "testing": true,
	"dummy": true, "fake": true, "default": true, "value": true, "values": true,
	"password": true, "passwd": true, "pwd": true, "secret": true,
	"yourpassword": true, "your-password": true, "your_password": true, "yourpass": true,
	"admin": true, "administrator": true, "root": true, "letmein": true, "welcome": true, "qwerty": true,
	"12345678": true, "123456789": true, "1234567890": true,
	"null": true, "nil": true, "none": true, "true": true, "false": true,
	"undefined": true, "unset": true, "empty": true,
}

// placeholderSubstrings are safe to match anywhere inside a value: real
// credentials essentially never contain these words.
var placeholderSubstrings = []string{"example", "changeme", "placeholder", "password", "passwd"}

func isPlaceholderValue(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" || placeholderValues[v] {
		return true
	}
	if strings.HasPrefix(v, "xxx") {
		return true
	}
	for _, s := range placeholderSubstrings {
		if strings.Contains(v, s) {
			return true
		}
	}
	trimmed := strings.Trim(v, "!?.;,:-_ ")
	trimmed = strings.TrimRight(trimmed, "0123456789")
	trimmed = strings.Trim(trimmed, "!?.;,:-_ ")
	return placeholderValues[trimmed]
}

// shannonEntropy measures the per-character entropy of s in bits. A uniform
// distribution of n distinct characters scores log2(n); repeated filler
// collapses toward 0.
func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := make(map[rune]int)
	total := 0
	for _, r := range s {
		counts[r]++
		total++
	}
	entropy := 0.0
	for _, count := range counts {
		p := float64(count) / float64(total)
		entropy -= p * math.Log2(p)
	}
	return entropy
}

// redactedPreview shows the first four characters of a secret plus an
// ellipsis. The full secret is never emitted into a finding.
func redactedPreview(secret string) string {
	runes := []rune(secret)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:4]) + "…"
}

// position translates a byte offset into a 1-based line and rune column.
// Columns count runes, not bytes, so a finding in a line with emoji or other
// multibyte text still points where a human reads it (same UTF-8-safety
// intent as biome's sourcePosition, computed in memory instead of re-reading
// the file).
func position(data []byte, offset int) (line, column int) {
	line, column = 1, 1
	for i := 0; i < offset && i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
		i += size
	}
	return line, column
}
