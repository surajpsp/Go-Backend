package handlers

import (
	"encoding/json"
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

// Health answers a liveness probe. Useful for checking from a phone or emulator
// that the backend is reachable at all before suspecting the app.
func Health(c *gin.Context) {
	response.OK(c, "service is healthy", gin.H{"status": "up"})
}

// bindMessage turns Gin's binding error into a message safe to show a user.
//
// Three cases: field validation errors become e.g. "price must be greater than
// 0"; a JSON type mismatch names the offending field; anything else (empty
// body, malformed JSON) gets a generic message rather than the decoder's raw
// text, which is either cryptic ("EOF") or leaks Go type names.
func bindMessage(err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		msgs := make([]string, 0, len(ve))
		for _, fe := range ve {
			field := fe.Field()
			switch fe.Tag() {
			case "required", "notblank":
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

	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) && ute.Field != "" {
		return ute.Field + " has the wrong type, expected " + ute.Type.String()
	}

	return "request body must be valid JSON"
}

// maxLimit caps an explicit ?limit= so one request can't be made to build an
// arbitrarily large response.
const maxLimit = 100

// List returns products, newest filters first:
//
//	GET /products?search=<q>&page=<n>&limit=<n>
//
// Pagination is opt-in. With neither page nor limit supplied the endpoint
// returns every matching product, which is what the API contract specifies;
// supplying either switches to a windowed response. The pagination block is
// present either way so a client can read it unguarded.
func (h *ProductHandler) List(c *gin.Context) {
	search := c.Query("search")

	pageStr, hasPage := c.GetQuery("page")
	limitStr, hasLimit := c.GetQuery("limit")
	paginated := hasPage || hasLimit

	page, limit := 1, 0 // limit 0 = no window
	if paginated {
		var err error
		if page, err = positiveQuery(pageStr, 1); err != nil {
			response.Error(c, http.StatusBadRequest, "validation_failed", "page must be a positive integer")
			return
		}
		if limit, err = positiveQuery(limitStr, maxLimit); err != nil {
			response.Error(c, http.StatusBadRequest, "validation_failed", "limit must be a positive integer")
			return
		}
		if limit > maxLimit {
			limit = maxLimit
		}
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

	meta := response.Pagination{Page: page, Limit: limit, Total: total, TotalPages: 1}
	if limit > 0 {
		meta.TotalPages = (total + limit - 1) / limit // ceil division
	} else {
		meta.Limit = total // the whole set was returned in one page
	}
	response.List(c, "products fetched successfully", items, meta)
}

// positiveQuery parses a query-string integer that must be >= 1. An empty
// string means the caller omitted it, in which case def is used.
func positiveQuery(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 0, errors.New("not a positive integer")
	}
	return n, nil
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
	// Store the trimmed values: notblank only proves there is a non-space
	// character, it doesn't strip the padding around it.
	p, err := h.store.Create(models.Product{
		Name:          strings.TrimSpace(req.Name),
		SKU:           strings.TrimSpace(req.SKU),
		Price:         req.Price,
		StockQuantity: req.StockQuantity,
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
	p, err := h.store.AdjustStock(id, *req.Quantity)
	switch {
	case errors.Is(err, store.ErrNotFound):
		response.Error(c, http.StatusNotFound, "not_found", "product not found")
	case errors.Is(err, store.ErrInsufficientStock):
		response.Error(c, http.StatusConflict, "insufficient_stock", "resulting stock cannot be negative")
	case errors.Is(err, store.ErrStockOverflow):
		response.Error(c, http.StatusBadRequest, "validation_failed", "quantity is too large")
	case err != nil:
		c.Error(err)
		response.Error(c, http.StatusInternalServerError, "internal_error", "could not adjust stock")
	default:
		response.OK(c, "stock updated successfully", p)
	}
}
