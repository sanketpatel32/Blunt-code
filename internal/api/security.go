package api

import (
	"net"
	"net/http"
	"strings"
)

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			fail(w, 421, "LOCAL_ONLY", "Blunt Code accepts loopback requests only.")
			return
		}
		// Every response — including mux-generated 404/405 text bodies that
		// bypass writeJSON — carries no-store, so the embedded UI never serves
		// a stale API or asset response from cache.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// frame-src blob: lets the report viewer frame a blob object URL; a blob's
		// origin is the app itself, so nothing foreign is framed.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; frame-src 'self' blob:")
		next.ServeHTTP(w, r)
	})
}
func loopbackHost(host string) bool {
	if strings.TrimSpace(host) == "" {
		return true
	}
	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
	}
	hostname = strings.Trim(hostname, "[]")
	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}
func isStateChanging(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}
