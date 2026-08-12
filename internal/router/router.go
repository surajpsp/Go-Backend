// Package router assembles the HTTP engine: middleware chain, routes, and the
// framework's own failure paths. It lives apart from main so tests can exercise
// the real stack — same middleware, same routes — against a test store.
package router

import (
	"github.com/gin-gonic/gin"

	"go-backend/internal/handlers"
	"go-backend/internal/middleware"
	"go-backend/internal/store"
)

// New builds the engine serving the product API against s.
func New(s store.Store) *gin.Engine {
	h := handlers.NewProductHandler(s)

	// gin.New (not gin.Default) so gin's own logger and its recovery — which
	// writes an empty body — are replaced by ones that log structurally and
	// answer with the JSON envelope.
	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.RequestLogger(),
		middleware.Recovery(),
		middleware.BodyLimit(middleware.MaxBodyBytes),
	)
	r.HandleMethodNotAllowed = true
	r.NoRoute(middleware.NotFound)
	r.NoMethod(middleware.MethodNotAllowed)

	// Liveness probe: cheap, unauthenticated, and useful for confirming the
	// server is reachable from a device or emulator before debugging the app.
	r.GET("/health", handlers.Health)

	products := r.Group("/products")
	{
		products.GET("", h.List)
		products.GET("/:id", h.Get)
		products.POST("", h.Create)
		products.POST("/:id/stock", h.AdjustStock)
	}
	return r
}
