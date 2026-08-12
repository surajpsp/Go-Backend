package store

import (
	"errors"

	"go-backend/internal/models"
)

var (
	ErrNotFound          = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrDuplicateSKU      = errors.New("sku already exists")
	ErrStockOverflow     = errors.New("stock adjustment overflows")
)

// ListParams carries the optional search filter and pagination window for List.
type ListParams struct {
	Search string // matched against name and sku (case-insensitive); empty = no filter
	Limit  int    // page size; <= 0 returns every matching row
	Offset int    // rows to skip = (page-1) * limit
}

// Store abstracts persistence. SQLite implements it now; Postgres could later.
type Store interface {
	// List returns the page of products plus the total count of matching rows
	// (before the limit/offset window is applied) so callers can paginate.
	List(p ListParams) (items []models.Product, total int, err error)
	Get(id int) (models.Product, error)
	Create(p models.Product) (models.Product, error)
	AdjustStock(id, delta int) (models.Product, error)
}
