package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// jsonRouteErrors gives the ServeMux's own "404 page not found" and
// "405 method not allowed" answers for /api/ paths the same JSON error
// envelope every handler uses, so clients can always read error.code.
// Handler-written responses (already JSON) pass through untouched.
func jsonRouteErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(&routeErrorWriter{ResponseWriter: w}, r)
	})
}

type routeErrorWriter struct {
	http.ResponseWriter
	replaced bool
}

func (w *routeErrorWriter) WriteHeader(code int) {
	contentType := w.Header().Get("Content-Type")
	if (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) && strings.HasPrefix(contentType, "text/plain") {
		w.replaced = true
		errCode, message := "NOT_FOUND", "This API route does not exist."
		if code == http.StatusMethodNotAllowed {
			errCode, message = "METHOD_NOT_ALLOWED", "This API route does not accept that method."
		}
		h := w.Header()
		h.Del("Content-Length")
		h.Del("X-Content-Type-Options")
		h.Set("Content-Type", "application/json")
		h.Set("X-Content-Type-Options", "nosniff")
		w.ResponseWriter.WriteHeader(code)
		_ = json.NewEncoder(w.ResponseWriter).Encode(map[string]any{"error": map[string]string{"code": errCode, "message": message}})
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *routeErrorWriter) Write(b []byte) (int, error) {
	if w.replaced {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

// Flush keeps server-sent events (scan progress) streaming through the wrapper.
func (w *routeErrorWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *routeErrorWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
