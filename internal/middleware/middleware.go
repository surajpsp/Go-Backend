// Package middleware wires the framework's own failure paths — panics, unknown
// routes, wrong methods — into the same JSON envelope the handlers use, so a
// client never has to parse an HTML error page or an empty body. It also logs
// every request that passes through.
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"

	"go-backend/internal/response"
)

// RequestID tags each request with a short id, echoes it back in the
// X-Request-ID header, and makes it available to the logger and the error
// envelope — so an id a client reports maps straight to a log line.
//
// An inbound X-Request-ID is reused only if it looks like an id we would have
// generated; anything else is replaced rather than trusted into our logs.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if !validID(id) {
			id = newID()
		}
		c.Set(response.CtxRequestID, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is not worth killing a request over; the id is
		// only a correlation aid, so fall back to the clock.
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(b)
}

// validID keeps client-supplied ids to a short, boring alphabet so nothing odd
// reaches a log file or a response header.
func validID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// RequestLogger writes one structured line per request after it completes.
// 5xx logs at error level, 4xx at warn, everything else at info.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		// Captured before Next: a handler may rewrite the URL.
		path, query := c.Request.URL.Path, c.Request.URL.RawQuery

		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"requestId", c.GetString(response.CtxRequestID),
			"method", c.Request.Method,
			"path", path,
			"status", status,
			"latencyMs", float64(time.Since(start).Microseconds()) / 1000,
			"ip", c.ClientIP(),
			"bytes", c.Writer.Size(),
		}
		if query != "" {
			attrs = append(attrs, "query", query)
		}
		if code := c.GetString(response.CtxErrorCode); code != "" {
			attrs = append(attrs, "errorCode", code)
		}
		// Underlying causes handlers attached with c.Error — the real DB or
		// driver message, which the client is never shown.
		if len(c.Errors) > 0 {
			attrs = append(attrs, "cause", c.Errors.String())
		}

		switch {
		case status >= http.StatusInternalServerError:
			slog.Error("request", attrs...)
		case status >= http.StatusBadRequest:
			slog.Warn("request", attrs...)
		default:
			slog.Info("request", attrs...)
		}
	}
}

// Recovery turns a panic in any handler into a 500 envelope. The panic value and
// stack are logged server-side only; the client gets a generic message so
// internal detail (paths, SQL, stack) never leaks.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		slog.Error("panic recovered",
			"requestId", c.GetString(response.CtxRequestID),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"panic", recovered,
			"stack", string(debug.Stack()),
		)
		response.Error(c, http.StatusInternalServerError, "internal_error", "something went wrong, please try again")
	})
}

// NotFound handles requests to paths that match no route.
func NotFound(c *gin.Context) {
	response.Error(c, http.StatusNotFound, "route_not_found", "the requested endpoint does not exist")
}

// MethodNotAllowed handles a known path called with the wrong HTTP method.
// Requires gin's HandleMethodNotAllowed to be enabled on the engine.
func MethodNotAllowed(c *gin.Context) {
	response.Error(c, http.StatusMethodNotAllowed, "method_not_allowed", "this method is not allowed on the requested endpoint")
}
