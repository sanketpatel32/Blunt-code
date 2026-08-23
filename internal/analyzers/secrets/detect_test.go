package secrets

// Pure detector tests. The inputs double as fixtures proving that
// word-boundary and format-strict matching keep prose mentions and pattern
// descriptions from firing.

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func rulesOf(ms []match) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.rule)
	}
	return out
}

func TestStructuredDetection(t *testing.T) {
	githubToken := "ghp_" + strings.Repeat("a9ZzQx1p", 4) + "Lm4o" // 36 payload characters
	slackToken := "xoxb-" + "Ab12Cd34Ef56Gh78Ij90"
	googleKey := "AIza" + strings.Repeat("a1B2c3D4e5", 3) + "f6G7h" // 35 payload characters
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2-QT4fwpMeJf36POk6yJV_adQssw5c"
	if payload := len(githubToken) - len("ghp_"); payload != 36 {
		t.Fatalf("github token fixture has %d payload characters, want 36", payload)
	}
	if payload := len(googleKey) - len("AIza"); payload != 35 {
		t.Fatalf("google key fixture has %d payload characters, want 35", payload)
	}
	cases := []struct {
		name      string
		input     string
		wantRules []string
	}{
		// AWS access key IDs.
		{"aws positive quoted", `aws_key = "AKIA1234567890ABCDEF"`, []string{ruleAWSAccessKey}},
		{"aws positive bare", "AKIA1234567890ABCDEF", []string{ruleAWSAccessKey}},
		{"aws near-miss truncated", "AKIA1234567890ABCDE", nil},
		{"aws near-miss lowercase prefix", "akia1234567890ABCDEF", nil},
		{"aws near-miss embedded in longer token", "XAKIA1234567890ABCDEF", nil},
		{"aws near-miss trailing extra character", "AKIA1234567890ABCDEFx", nil},
		{"aws prose mention", "The AKIA prefix marks an AWS access key ID in docs.", nil},
		// GitHub tokens.
		{"github positive", "curl -H \"Authorization: token " + githubToken + "\" https://api.github.com", []string{ruleGitHubToken}},
		{"github near-miss truncated", "ghp_" + strings.Repeat("a9ZzQx1p", 4) + "Lm4", nil},
		{"github near-miss wrong prefix", "ghx_" + strings.Repeat("a9ZzQx1p", 4) + "Lm4o", nil},
		{"github prose mention", "GitHub tokens use the ghp_ prefix with 36 characters.", nil},
		// Slack tokens.
		{"slack positive", `slack_token = "` + slackToken + `"`, []string{ruleSlackToken}},
		{"slack near-miss truncated", "xoxb-1234", nil},
		{"slack near-miss wrong type", "xoxz-Ab12Cd34Ef56Gh78Ij90", nil},
		// Google API keys.
		{"google positive", "maps_key: " + googleKey, []string{ruleGoogleAPIKey}},
		{"google near-miss truncated", "AIza" + strings.Repeat("a1B2c3D4e5", 3) + "f6G", nil},
		{"google near-miss wrong prefix", "AIzb" + strings.Repeat("a1B2c3D4e5", 3) + "f6G7h", nil},
		// Private key blocks.
		{"private key rsa", "-----BEGIN RSA PRIVATE KEY-----", []string{rulePrivateKeyBlock}},
		{"private key ec", "-----BEGIN EC PRIVATE KEY-----", []string{rulePrivateKeyBlock}},
		{"private key openssh", "-----BEGIN OPENSSH PRIVATE KEY-----", []string{rulePrivateKeyBlock}},
		{"private key untyped", "-----BEGIN PRIVATE KEY-----", []string{rulePrivateKeyBlock}},
		{"private key near-miss public", "-----BEGIN PUBLIC KEY-----", nil},
		{"private key near-miss dsa unsupported", "-----BEGIN DSA PRIVATE KEY-----", nil},
		{"private key near-miss truncated", "-----BEGIN PRIVATE KEY", nil},
		{"private key near-miss pgp block suffix", "-----BEGIN PGP PRIVATE KEY BLOCK-----", nil},
		// JWTs.
		{"jwt positive", `session = "` + jwt + `"`, []string{ruleJWT}},
		{"jwt near-miss two segments", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0", nil},
		{"jwt near-miss missing header prefix", "hbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", nil},
		{"jwt near-miss embedded in word", "x" + jwt, nil},
		// Connection URIs with credentials.
		{"uri postgresql positive", `DATABASE_URL = "postgresql://deploy:hV9kLm2Qr7@db.example.com/prod"`, []string{ruleConnectionURI}},
		{"uri redis positive", "redis://default:S3cr3tSup3r@redis.internal:6379", []string{ruleConnectionURI}},
		{"uri amqp positive", `broker = "amqp://svc:Passw0rd9@mq.internal:5672/%2Fvhost"`, []string{ruleConnectionURI}},
		{"uri mongodb srv positive", "mongodb+srv://svc:LongPassphrase1@cluster.example.net", []string{ruleConnectionURI}},
		{"uri https positive", "https://deploy:Hunt3r2Hunter@ci.example.com/job/1", []string{ruleConnectionURI}},
		{"uri near-miss no password", "postgres://application@db.internal", nil},
		{"uri near-miss placeholder pass", "postgres://user:pass@example.com", nil},
		{"uri near-miss placeholder password", "postgres://user:password@example.com", nil},
		{"uri near-miss no credentials", "http://localhost:8080/health", nil},
		{"uri near-miss unsupported scheme", "ftp://user:secretpw@files.example.com", nil},
		{"uri prose sample scheme", "see scheme://user:password@host in the README", nil},
		{"uri near-miss scheme inside word", "xdbpostgres://user:secretpw@db.internal", nil},
		// A structured value inside a generic assignment reports only the
		// structured rule, never a duplicate generic finding.
		{"structured value suppresses generic", `token = "AKIA1234567890ABCDEF"`, []string{ruleAWSAccessKey}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := rulesOf(detect([]byte(c.input)))
			if len(got) != len(c.wantRules) {
				t.Fatalf("detect(%q) rules = %v, want %v", c.input, got, c.wantRules)
			}
			for i := range got {
				if got[i] != c.wantRules[i] {
					t.Fatalf("detect(%q) rules = %v, want %v", c.input, got, c.wantRules)
				}
			}
		})
	}
}

func TestGenericAssignmentDetection(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantFire bool
	}{
		{"real password double quoted", `password = "hV9kLm2Qr7StZx"`, true},
		{"underscore compound key", `DB_PASSWORD='Zx91plQw87er'`, true},
		{"json api key", `{"api_key": "sk-proj-4f8b2c91a77e"}`, true},
		{"yaml password", `password: "Zx91plQw87er"`, true},
		{"go walrus token", `token := "dfe3a91b77214f0c"`, true},
		{"auth token with spaces in value", `auth_token = "Bearer 0perationa1"`, true},
		{"kebab-case key", `db-password: "Plqw87erZx91m"`, true},
		{"access key env style", `ACCESS_KEY = "F8s2K1lQ9wZx3Rt5"`, true},
		{"secret single quoted", `secret = 'M4k3-1t-s0-much-harder'`, true},

		{"placeholder changeme", `password = "changeme"`, false},
		{"placeholder example", `password = "example"`, false},
		{"placeholder example substring", `api_key = "sk-example-001122"`, false},
		{"placeholder xxx run", `password = "xxxxxxxxxxxx"`, false},
		{"placeholder word", `password = "placeholder"`, false},
		{"placeholder todo with digits", `password = "todo1234"`, false},
		{"placeholder test with digits", `password = "test1234"`, false},
		{"placeholder password123", `password = "password123"`, false},
		{"placeholder weak word", `secret = "letmein12345"`, false},
		{"too short", `password = "hunter2"`, false},
		{"low entropy repeated", `password = "AAAAAAAAAA"`, false},
		{"low entropy plain word", `password = "hunter2hunter2"`, false},
		{"unquoted value", `password = hunter2hunter2`, false},
		{"key not in list", `username = "administrator"`, false},
		{"key boundary inside word", `myPassword = "Zx91plQw87er"`, false},
		{"key boundary compound go name", `let secretCount = 10`, false},
		{"prose without quoted value", "// remember to set the password before shipping", false},
		{"struct tag not an assignment", "`json:\"password\"`", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fired := false
			for _, m := range detect([]byte(c.input)) {
				if m.rule == ruleGenericAssign {
					fired = true
				}
			}
			if fired != c.wantFire {
				t.Fatalf("generic rule fired = %v for %q, want %v", fired, c.input, c.wantFire)
			}
		})
	}
}

func TestGenericMatchPointsAtValue(t *testing.T) {
	input := `password = "hV9kLm2Qr7StZx"`
	ms := detect([]byte(input))
	if len(ms) != 1 {
		t.Fatalf("detect returned %d matches, want 1: %v", len(ms), ms)
	}
	m := ms[0]
	if want := strings.Index(input, "hV9k"); m.start != want {
		t.Fatalf("match start = %d, want %d (the value, not the key)", m.start, want)
	}
	if m.secret != "hV9kLm2Qr7StZx" {
		t.Fatalf("match secret = %q, want the assigned value", m.secret)
	}
	if m.key != "password" {
		t.Fatalf("match key = %q, want %q", m.key, "password")
	}
}

func TestShannonEntropy(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"", 0},
		{"aaaaaaaa", 0},
		{"ab", 1},
		{"abcdefgh", 3},                   // eight distinct characters over eight runes
		{"password", 2.75},                // one repeated rune (s) — plain word territory
		{"test1234", 2.75},                // two repeats; below the floor of 3
		{"hV9kLm2Qr7StZx", math.Log2(14)}, // all distinct
	}
	for _, c := range cases {
		got := shannonEntropy(c.input)
		if math.Abs(got-c.want) > 1e-9 {
			t.Fatalf("shannonEntropy(%q) = %v, want %v", c.input, got, c.want)
		}
	}
	// The floor itself: high-entropy values pass, plain filler does not.
	if shannonEntropy("dfe3a91b77214f0c") < minEntropyBits {
		t.Fatal("a real-looking secret fell below the entropy floor")
	}
}

func TestPlaceholderValues(t *testing.T) {
	for _, v := range []string{"changeme", "CHANGEME", "changeme!", "todo1234", "test1234", "xxxxxxxx", "example", "some-example-value", "placeholder", "your-password", "password123"} {
		if !isPlaceholderValue(v) {
			t.Fatalf("isPlaceholderValue(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"hV9kLm2Qr7StZx", "Zx91plQw87er", "sk-proj-4f8b2c91a77e", "Passw0rd9", "LongPassphrase1", "mastodon-api-42"} {
		if isPlaceholderValue(v) {
			t.Fatalf("isPlaceholderValue(%q) = true, want false", v)
		}
	}
}

func TestRedactedPreview(t *testing.T) {
	cases := []struct {
		secret, want string
	}{
		{"AKIA1234567890ABCDEF", "AKIA…"},
		{"hV9kLm2Qr7StZx", "hV9k…"},
		{"🎉🎉🎉🎉🎉", "🎉🎉🎉🎉…"},
		{"tiny", "****"},
	}
	for _, c := range cases {
		if got := redactedPreview(c.secret); got != c.want {
			t.Fatalf("redactedPreview(%q) = %q, want %q", c.secret, got, c.want)
		}
	}
	// The preview must never contain more than the first four characters.
	secret := "dfe3a91b77214f0c"
	if preview := redactedPreview(secret); strings.Contains(preview, secret[5:]) {
		t.Fatalf("preview %q leaks the secret body", preview)
	}
}

func TestPositionCountsRunesNotBytes(t *testing.T) {
	// One 4-byte emoji up front: byte offsets and rune columns diverge.
	data := []byte("\"🎉\"; token = \"x\"")
	// Rune 14 is the opening quote of the value: bytes 0(quote),1-4(emoji),5(quote),6(;),7(space),8-12(token),13(space),14(=),15(space),16(quote).
	if line, col := position(data, 16); line != 1 || col != 14 {
		t.Fatalf("position(offset 16) = (%d,%d), want (1,14)", line, col)
	}
	if line, col := position(data, 5); line != 1 || col != 3 {
		t.Fatalf("position(offset 5) = (%d,%d), want (1,3)", line, col)
	}
	twoLines := []byte("first\n🎉 second")
	if line, col := position(twoLines, 6); line != 2 || col != 1 {
		t.Fatalf("position(offset 6) = (%d,%d), want (2,1)", line, col)
	}
	if line, col := position(twoLines, 10); line != 2 || col != 2 {
		t.Fatalf("position(offset 10) = (%d,%d), want (2,2)", line, col)
	}
}

func TestPerFileCapInsideScanFile(t *testing.T) {
	var lines []string
	for i := 0; i < maxFindingsPerFile+10; i++ {
		lines = append(lines, fmt.Sprintf("key%d = %q", i, fmt.Sprintf("AKIA%016X", i)))
	}
	diagnostics, truncated := scanFile("cap.go", []byte(strings.Join(lines, "\n")))
	if !truncated {
		t.Fatal("scanFile did not report truncation past the per-file cap")
	}
	if len(diagnostics) != maxFindingsPerFile {
		t.Fatalf("scanFile returned %d diagnostics, want the cap of %d", len(diagnostics), maxFindingsPerFile)
	}
}
