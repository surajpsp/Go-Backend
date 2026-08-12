package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"go-backend/internal/models"
	"go-backend/internal/response"
	"go-backend/internal/store"
)

type ProductHandler struct{ store store.Store }

func NewProductHandler(s store.Store) *ProductHandler { return &ProductHandler{store: s} }

// bindMessage turns Gin's binding error into a human-readable message. Field
// validation errors become e.g. "price must be greater than 0"; a malformed
// JSON body (not a validator error) falls back to the raw parse message.
func bindMessage(err error) string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err.Error()
	}
	msgs := make([]string, 0, len(ve))
	for _, fe := range ve {
		field := fe.Field()
		switch fe.Tag() {
		case "required":
			msgs = append(msgs, field+" is required")
		case "gt":
			msgs = append(msgs, field+" must be greater than "+fe.Param())
		case "gte":
			msgs = append(msgs, field+" must be at least "+fe.Param())
		default:
			msgs = append(msgs, field+" is invalid")
		}
	}
	return strings.Join(msgs, "; ")
}

// Pagination defaults and guardrails.
const (
	defaultLimit = 10
	maxLimit     = 100
)

// List supports optional search and pagination via query params:
//
//	GET /products?search=<q>&page=<n>&limit=<n>
func (h *ProductHandler) List(c *gin.Context) {
	search := c.Query("search")
	page := atoiDefault(c.Query("page"), 1)
	if page < 1 {
		page = 1
	}
	limit := atoiDefault(c.Query("limit"), defaultLimit)
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit // cap so a client can't request the whole table at once
	}

	items, total, err := h.store.List(store.ListParams{
		Search: search,
		Limit:  limit,
		Offset: (page - 1) * limit,
	})
	if err != nil {
		c.Error(err) // real cause goes to the log, not to the client
		response.Error(c, http.StatusInternalServerError, "internal_error", "could not fetch products")
		return
	}
	if items == nil {
		items = []models.Product{} // never send `null` where the client expects a list
	}

	totalPages := (total + limit - 1) / limit // ceil division
	response.List(c, "products fetched successfully", items, response.Pagination{
		Page: page, Limit: limit, Total: total, TotalPages: totalPages,
	})
}

// atoiDefault parses s as an int, returning def if s is empty or not a number.
func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func (h *ProductHandler) Get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a number")
		return
	}
	p, err := h.store.Get(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "product not found")
	case err != nil:
		c.Error(err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "could not fetch product")
	default:
		response.OK(c, "product fetched successfully", p)
	}
}

func (h *ProductHandler) Create(c *gin.Context) {
	var req models.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "validation_failed", bindMessage(err))
		return
	}
	p, err := h.store.Create(models.Product{
		Name: req.Name, SKU: req.SKU, Price: req.Price, StockQuantity: req.StockQuantity,
	})
	switch {
	case errors.Is(err, store.ErrDuplicateSKU):
		response.Error(c, http.StatusConflict, "duplicate_sku", "a product with this sku already exists")
	case err != nil:
		c.Error(err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "could not create product")
	default:
		response.Created(c, "product created successfully", p)
	}
}

func (h *ProductHandler) AdjustStock(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid_id", "id must be a number")
		return
	}
	var req models.AdjustStockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "validation_failed", bindMessage(err))
		return
	}
	p, err := h.store.AdjustStock(id, req.Quantity)
	switch {
	case errors.Is(err, store.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "product not found")
	case errors.Is(err, store.ErrInsufficientStock):
		response.Error(c, http.StatusConflict, "insufficient_stock", "resulting stock cannot be negative")
	case err != nil:
		c.Error(err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "could not adjust stock")
	default:
		response.OK(c, "stock updated successfully", p)
	}
}
