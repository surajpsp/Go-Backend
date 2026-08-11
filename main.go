package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"go-backend/internal/handlers"
	"go-backend/internal/store"
)

func main() {
	s, err := store.NewSQLite("products.db")
	if err != nil {
		log.Fatalf("failed to init store: %v", err)
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
