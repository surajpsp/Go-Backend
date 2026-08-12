// Package seed populates the store with a fixed set of demo products.
package seed

import (
	"errors"
	"fmt"

	"go-backend/internal/models"
	"go-backend/internal/store"
)

// baseItem is one product line; each is stocked in every colorway below, so the
// catalog is len(baseItems) * len(colorways) products.
type baseItem struct {
	name      string
	skuPrefix string
	price     float64
	stock     int // stock of the first colorway; later ones step down from here
}

var baseItems = []baseItem{
	{"Mechanical Keyboard", "KB-MECH-87", 4499.00, 25},
	{"Wireless Mouse", "MS-WRL-01", 1299.50, 80},
	{"27\" 4K Monitor", "MON-4K-27", 28999.00, 12},
	{"USB-C Hub 7-in-1", "HUB-USBC-7", 2499.00, 45},
	{"Noise Cancelling Headphones", "HP-ANC-900", 18999.00, 18},
	{"Laptop Stand Aluminium", "STD-ALU-01", 1899.00, 60},
	{"1080p Webcam", "CAM-1080-P", 3299.00, 30},
	{"Portable SSD 1TB", "SSD-EXT-1T", 8999.00, 22},
	{"Ergonomic Chair", "CHR-ERG-77", 32999.00, 7},
	{"Desk Mat XL", "MAT-DSK-XL", 999.00, 9},
	{"Mechanical Numpad", "KB-NUM-21", 1799.00, 40},
	{"Cable Organizer Pack", "ORG-CBL-05", 449.00, 150},
}

// colorway is a variant of a base item: a name suffix, a SKU suffix, and a
// price premium over the base price.
type colorway struct {
	label   string
	code    string
	premium float64
}

var colorways = []colorway{
	{"Black", "BLK", 0},
	{"White", "WHT", 0},
	{"Graphite", "GRP", 100},
	{"Silver", "SLV", 100},
	{"Midnight Blue", "MBL", 150},
	{"Forest Green", "FGR", 150},
	{"Sand", "SND", 200},
	{"Crimson", "CRM", 200},
	{"Slate", "SLT", 250},
	{"Rose Gold", "RSG", 500},
}

// products is the demo dataset, 120 rows. SKUs are unique, which is what makes
// re-running the seeder safe (see Run).
var products = buildCatalog()

// buildCatalog crosses every base item with every colorway. It is deterministic
// — same order, SKUs, prices and stock on every run — so seeded data is stable
// across machines and reruns.
func buildCatalog() []models.Product {
	out := make([]models.Product, 0, len(baseItems)*len(colorways))
	for _, b := range baseItems {
		for i, c := range colorways {
			// Stock steps down across colorways so the data isn't uniform;
			// every 7th variant lands on 0 to exercise the insufficient_stock path.
			stock := b.stock - i*2
			if stock < 0 || i%7 == 6 {
				stock = 0
			}
			out = append(out, models.Product{
				Name:          fmt.Sprintf("%s (%s)", b.name, c.label),
				SKU:           b.skuPrefix + "-" + c.code,
				Price:         b.price + c.premium,
				StockQuantity: stock,
			})
		}
	}
	return out
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

// Total reports how many products the seed dataset contains.
func Total() int { return len(products) }
