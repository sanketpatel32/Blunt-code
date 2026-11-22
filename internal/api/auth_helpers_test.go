package api

import "net/http"

// testHandler is the API handler as the shipped UI reaches it: every request
// carries the server's bearer token, and a JSON body is labelled as such.
// Tests that exercise the guard itself use s.Handler() directly.
func testHandler(s *Server) http.Handler {
	inner := s.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			r.Header.Set("Authorization", "Bearer "+s.Token())
		}
		if r.Header.Get("Content-Type") == "" && hasBody(r) {
			r.Header.Set("Content-Type", "application/json")
		}
		inner.ServeHTTP(w, r)
	})
}
