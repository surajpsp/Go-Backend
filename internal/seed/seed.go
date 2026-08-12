// Package seed populates the store with a fixed set of demo products.
package seed

import (
	"errors"
	"log"

	"go-backend/internal/models"
	"go-backend/internal/store"
)

// products is the fixed demo dataset. SKUs are unique, which is what makes
// re-running the seeder safe (see Run).
var products = []models.Product{
	{Name: "Mechanical Keyboard", SKU: "KB-MECH-87", Price: 4499.00, StockQuantity: 25},
	{Name: "Wireless Mouse", SKU: "MS-WRL-01", Price: 1299.50, StockQuantity: 80},
	{Name: "27\" 4K Monitor", SKU: "MON-4K-27", Price: 28999.00, StockQuantity: 12},
	{Name: "USB-C Hub 7-in-1", SKU: "HUB-USBC-7", Price: 2499.00, StockQuantity: 45},
	{Name: "Noise Cancelling Headphones", SKU: "HP-ANC-900", Price: 18999.00, StockQuantity: 18},
	{Name: "Laptop Stand Aluminium", SKU: "STD-ALU-01", Price: 1899.00, StockQuantity: 60},
	{Name: "1080p Webcam", SKU: "CAM-1080-P", Price: 3299.00, StockQuantity: 30},
	{Name: "Portable SSD 1TB", SKU: "SSD-EXT-1T", Price: 8999.00, StockQuantity: 22},
	{Name: "Ergonomic Chair", SKU: "CHR-ERG-77", Price: 32999.00, StockQuantity: 7},
	{Name: "Desk Mat XL", SKU: "MAT-DSK-XL", Price: 999.00, StockQuantity: 0},
	{Name: "Mechanical Numpad", SKU: "KB-NUM-21", Price: 1799.00, StockQuantity: 40},
	{Name: "Cable Organizer Pack", SKU: "ORG-CBL-05", Price: 449.00, StockQuantity: 150},
}

// Run inserts the demo products, skipping any whose SKU is already present.
// This makes it safe to run on every boot: existing rows (including any stock
// adjustments made since) are left untouched. It returns the number inserted.
func Run(s store.Store) (int, error) {
	inserted := 0
	for _, p := range products {
		if _, err := s.Create(p); err != nil {
			if errors.Is(err, store.ErrDuplicateSKU) {
				continue
			}
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

// MustRun runs the seeder and logs the outcome, exiting on failure.
func MustRun(s store.Store) {
	n, err := Run(s)
	if err != nil {
		log.Fatalf("seed failed: %v", err)
	}
	log.Printf("seed: inserted %d product(s), skipped %d already present", n, len(products)-n)
}
