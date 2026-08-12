package main

import (
	"flag"
	"log"

	"github.com/gin-gonic/gin"

	"go-backend/internal/handlers"
	"go-backend/internal/seed"
	"go-backend/internal/store"
)

func main() {
	seedFlag := flag.Bool("seed", false, "insert demo products before starting (skips existing SKUs)")
	seedOnly := flag.Bool("seed-only", false, "insert demo products and exit without starting the server")
	flag.Parse()

	s, err := store.NewSQLite("products.db")
	if err != nil {
		log.Fatalf("failed to init store: %v", err)
	}

	if *seedFlag || *seedOnly {
		seed.MustRun(s)
		if *seedOnly {
			return
		}
	}

	h := handlers.NewProductHandler(s)

	r := gin.Default()
	products := r.Group("/products")
	{
		products.GET("", h.List)
		products.GET("/:id", h.Get)
		products.POST("", h.Create)
		products.POST("/:id/stock", h.AdjustStock)
	}

	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
