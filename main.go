package main

import (
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"go-backend/internal/handlers"
	"go-backend/internal/logger"
	"go-backend/internal/middleware"
	"go-backend/internal/seed"
	"go-backend/internal/store"
)

func main() {
	seedFlag := flag.Bool("seed", false, "insert demo products before starting (skips existing SKUs)")
	seedOnly := flag.Bool("seed-only", false, "insert demo products and exit without starting the server")
	logDir := flag.String("log-dir", "logs", "directory for the daily JSON log files")
	flag.Parse()

	logFile, err := logger.Init(*logDir)
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer logFile.Close()

	s, err := store.NewSQLite("products.db")
	if err != nil {
		// Not log.Fatalf: slog.SetDefault routes the standard logger through
		// slog at info level, which would file a fatal error under the wrong
		// severity. Log at error level, then exit.
		slog.Error("failed to init store", "err", err)
		logFile.Close()
		os.Exit(1)
	}

	if *seedFlag || *seedOnly {
		seed.MustRun(s)
		if *seedOnly {
			return
		}
	}

	h := handlers.NewProductHandler(s)

	// gin.New (not gin.Default) so gin's own logger and its recovery — which
	// writes an empty body — are replaced by ones that log structurally and
	// answer with the JSON envelope.
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.RequestLogger(), middleware.Recovery())
	r.HandleMethodNotAllowed = true
	r.NoRoute(middleware.NotFound)
	r.NoMethod(middleware.MethodNotAllowed)

	products := r.Group("/products")
	{
		products.GET("", h.List)
		products.GET("/:id", h.Get)
		products.POST("", h.Create)
		products.POST("/:id/stock", h.AdjustStock)
	}

	slog.Info("server starting", "addr", ":8080", "logDir", *logDir)
	if err := r.Run(":8080"); err != nil {
		slog.Error("server failed", "err", err)
		logFile.Close()
		os.Exit(1)
	}
}
