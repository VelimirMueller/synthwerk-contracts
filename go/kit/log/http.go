package log

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// RequestIDHeader carries the request id in both directions.
const RequestIDHeader = "X-Request-ID"

var requestIDRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

// probePaths log at DEBUG only.
var probePaths = map[string]bool{"/healthz": true, "/readyz": true}

// HTTP writes one request line per request ("request completed") and sets the
// X-Request-ID response header. It keeps a caller id that matches
// [A-Za-z0-9_.-]{1,64}, else it makes a new one. /healthz and /readyz log at DEBUG.
//
// Wrap it inside the OTel middleware so the line carries the trace id:
// otelhttp.NewHandler(log.HTTP(logger, mux), "server").
func HTTP(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := r.Header.Get(RequestIDHeader)
		if !requestIDRe.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		level := slog.LevelInfo
		if probePaths[r.URL.Path] {
			level = slog.LevelDebug
		}
		attrs := []slog.Attr{
			slog.String("event.name", "http.request.completed"),
			slog.String("synthwerk.request_id", id),
			slog.String("http.request.method", r.Method),
			slog.String("http.route", route(r)),
			slog.Int("http.response.status_code", rec.status),
			slog.Float64("synthwerk.duration_ms", float64(time.Since(start).Microseconds())/1000),
		}
		if addr := clientAddress(r.RemoteAddr); addr != "" {
			attrs = append(attrs, slog.String("client.address", addr))
		}
		logger.LogAttrs(r.Context(), level, "request completed", attrs...)
	})
}

// route returns the matched ServeMux pattern without the method and host,
// never the raw path (it can hold ids). Unmatched requests log "unmatched".
func route(r *http.Request) string {
	p := r.Pattern
	if p == "" {
		return "unmatched"
	}
	if i := strings.IndexByte(p, ' '); i >= 0 {
		p = strings.TrimSpace(p[i+1:])
	}
	if i := strings.IndexByte(p, '/'); i > 0 {
		p = p[i:] // drop a host part
	}
	return p
}

// clientAddress truncates IPv4 to /24 and IPv6 to /48.
func clientAddress(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	ip = ip.Unmap()
	bits := 24
	if ip.Is6() {
		bits = 48
	}
	prefix, err := ip.Prefix(bits)
	if err != nil {
		return ""
	}
	return prefix.Addr().String()
}

func newRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.wroteHeader = true
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush and deadlines (SSE in pulse).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
