package dast

import "testing"

func TestNormalizeTarget(t *testing.T) {
	cases := map[string]string{
		"localhost:3000":            "http://localhost:3000",
		"https://api.example.com/x": "https://api.example.com/x",
	}
	for in, want := range cases {
		got, err := NormalizeTarget(in)
		if err != nil || got.String() != want {
			t.Errorf("NormalizeTarget(%q) = %v, %v; want %s", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", "ftp://host", "http://"} {
		if _, err := NormalizeTarget(bad); err == nil {
			t.Errorf("NormalizeTarget(%q) accepted an invalid target", bad)
		}
	}
}

func TestIsLoopbackTarget(t *testing.T) {
	for _, in := range []string{"http://localhost:8080", "127.0.0.1:3000", "http://[::1]/", "http://127.5.5.5"} {
		if !IsLoopbackTarget(in) {
			t.Errorf("%s should be loopback", in)
		}
	}
	for _, in := range []string{"https://example.com", "http://10.0.0.1", "http://app.localhost", "http://localhost.evil.com", ""} {
		if IsLoopbackTarget(in) {
			t.Errorf("%s must not count as loopback", in)
		}
	}
}
