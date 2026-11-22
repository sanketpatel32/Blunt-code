package api

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			fail(w, 421, "LOCAL_ONLY", "Blunt Code accepts loopback requests only.")
			return
		}
		if isStateChanging(r.Method) && !safeOrigin(r) {
			fail(w, 403, "CROSS_ORIGIN_FORBIDDEN", "State-changing requests must come from the local Blunt Code origin.")
			return
		}
		// Every response — including mux-generated 404/405 text bodies that
		// bypass writeJSON — carries no-store, so the embedded UI never serves
		// a stale API or asset response from cache.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
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
func safeOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" {
		return false
	}
	return loopbackHost(u.Host)
}
