package models

type Product struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	SKU           string  `json:"sku"`
	Price         float64 `json:"price"`
	StockQuantity int     `json:"stockQuantity"`
}

type CreateProductRequest struct {
	Name          string  `json:"name"          binding:"required"`
	SKU           string  `json:"sku"           binding:"required"`
	Price         float64 `json:"price"         binding:"required,gt=0"`
	StockQuantity int     `json:"stockQuantity" binding:"gte=0"`
}

type AdjustStockRequest struct {
	Quantity int `json:"quantity" binding:"required"`
}

// Pagination is the metadata returned alongside a paginated list.
type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`      // total rows matching the (optional) search
	TotalPages int `json:"totalPages"` // ceil(total / limit)
}

// PaginatedProducts is the response shape for GET /products.
type PaginatedProducts struct {
	Data       []Product  `json:"data"`
	Pagination Pagination `json:"pagination"`
}
