package models

type Product struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	SKU           string  `json:"sku"`
	Price         float64 `json:"price"`
	StockQuantity int     `json:"stockQuantity"`
}

// CreateProductRequest is the POST /products body.
//
// Two tag choices are deliberate:
//
//	notblank — `required` on a string only rejects "", so "   " would pass and
//	           create a product with a blank name. notblank trims first.
//	price    — `required` is *not* used: for a numeric field it rejects the zero
//	           value, so {"price": 0} would fail as "price is required" rather
//	           than the real reason. gt=0 alone rejects both 0 and a missing
//	           price (which decodes to 0) with the correct message.
type CreateProductRequest struct {
	Name          string  `json:"name"          binding:"notblank"`
	SKU           string  `json:"sku"           binding:"notblank"`
	Price         float64 `json:"price"         binding:"gt=0"`
	StockQuantity int     `json:"stockQuantity" binding:"gte=0"`
}

// AdjustStockRequest is the POST /products/{id}/stock body.
//
// Quantity is a pointer so an absent field can be told apart from an explicit
// {"quantity": 0}. With a plain int, `required` rejects both, which would make
// a zero adjustment (a legal no-op) fail as "quantity is required".
type AdjustStockRequest struct {
	Quantity *int `json:"quantity" binding:"required"`
}
