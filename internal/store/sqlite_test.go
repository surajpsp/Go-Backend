package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"go-backend/internal/models"
)

// newTestStore returns a store backed by a fresh database file under the test's
// temp dir, removed automatically when the test ends.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustCreate(t *testing.T, s *SQLiteStore, name, sku string, price float64, stock int) models.Product {
	t.Helper()
	p, err := s.Create(models.Product{Name: name, SKU: sku, Price: price, StockQuantity: stock})
	if err != nil {
		t.Fatalf("Create(%s): %v", sku, err)
	}
	return p
}

func TestCreateAssignsIDAndRoundTrips(t *testing.T) {
	s := newTestStore(t)

	created := mustCreate(t, s, "Wireless Keyboard", "KB-001", 35.50, 10)
	if created.ID == 0 {
		t.Fatal("Create returned a zero ID")
	}

	got, err := s.Get(created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != created {
		t.Errorf("Get returned %+v, want %+v", got, created)
	}
}

func TestCreateRejectsDuplicateSKU(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "Keyboard", "KB-001", 35.50, 10)

	_, err := s.Create(models.Product{Name: "Other", SKU: "KB-001", Price: 1, StockQuantity: 1})
	if !errors.Is(err, ErrDuplicateSKU) {
		t.Fatalf("Create with duplicate sku: got %v, want ErrDuplicateSKU", err)
	}
}

func TestGetMissingReturnsErrNotFound(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.Get(404); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(404): got %v, want ErrNotFound", err)
	}
}

func TestAdjustStock(t *testing.T) {
	tests := []struct {
		name    string
		start   int
		delta   int
		want    int
		wantErr error
	}{
		{name: "adds stock", start: 10, delta: 5, want: 15},
		{name: "removes stock", start: 10, delta: -2, want: 8},
		{name: "zero is a no-op", start: 10, delta: 0, want: 10},
		{name: "may reach exactly zero", start: 10, delta: -10, want: 0},
		{name: "may not go negative", start: 10, delta: -11, wantErr: ErrInsufficientStock},
		{name: "rejects overflow", start: 10, delta: 1<<62 + 1<<62 - 1, wantErr: ErrStockOverflow},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			p := mustCreate(t, s, "Item", "SKU-1", 1, tc.start)

			got, err := s.AdjustStock(p.ID, tc.delta)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("AdjustStock: got %v, want %v", err, tc.wantErr)
				}
				// A rejected adjustment must not have touched the row.
				after, _ := s.Get(p.ID)
				if after.StockQuantity != tc.start {
					t.Errorf("stock changed on a rejected adjustment: %d, want %d", after.StockQuantity, tc.start)
				}
				return
			}
			if err != nil {
				t.Fatalf("AdjustStock: %v", err)
			}
			if got.StockQuantity != tc.want {
				t.Errorf("returned stock %d, want %d", got.StockQuantity, tc.want)
			}
			// The returned value must match what was persisted.
			after, err := s.Get(p.ID)
			if err != nil {
				t.Fatalf("Get after adjust: %v", err)
			}
			if after.StockQuantity != tc.want {
				t.Errorf("persisted stock %d, want %d", after.StockQuantity, tc.want)
			}
			if got != after {
				t.Errorf("returned %+v but stored %+v", got, after)
			}
		})
	}
}

func TestAdjustStockMissingProduct(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.AdjustStock(404, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AdjustStock on missing id: got %v, want ErrNotFound", err)
	}
}

// Concurrent decrements must never oversell: the transaction serialises the
// read-check-write, so exactly `start` units can be taken and no more.
func TestAdjustStockConcurrentDecrementsDoNotOversell(t *testing.T) {
	s := newTestStore(t)
	const start, attempts = 20, 50
	p := mustCreate(t, s, "Item", "SKU-1", 1, start)

	var wg sync.WaitGroup
	ok := make(chan struct{}, attempts)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.AdjustStock(p.ID, -1); err == nil {
				ok <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(ok)

	if got := len(ok); got != start {
		t.Errorf("%d decrements succeeded, want %d", got, start)
	}
	after, err := s.Get(p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if after.StockQuantity != 0 {
		t.Errorf("final stock %d, want 0", after.StockQuantity)
	}
}

func TestListReturnsEverythingWhenUnwindowed(t *testing.T) {
	s := newTestStore(t)
	for _, sku := range []string{"A-1", "B-2", "C-3"} {
		mustCreate(t, s, "Item "+sku, sku, 1, 1)
	}

	items, total, err := s.List(ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Errorf("got %d items / total %d, want 3 / 3", len(items), total)
	}
}

func TestListEmptyStoreReturnsEmptySlice(t *testing.T) {
	s := newTestStore(t)

	items, total, err := s.List(ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items == nil {
		t.Error("List returned a nil slice; it must encode as [] not null")
	}
	if total != 0 || len(items) != 0 {
		t.Errorf("got %d items / total %d, want 0 / 0", len(items), total)
	}
}

func TestListPaginationWindowsRowsButCountsAll(t *testing.T) {
	s := newTestStore(t)
	for _, sku := range []string{"A-1", "B-2", "C-3", "D-4", "E-5"} {
		mustCreate(t, s, "Item "+sku, sku, 1, 1)
	}

	items, total, err := s.List(ListParams{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5 (the count must ignore the window)", total)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].SKU != "C-3" || items[1].SKU != "D-4" {
		t.Errorf("got %s,%s want C-3,D-4", items[0].SKU, items[1].SKU)
	}
}

func TestListSearch(t *testing.T) {
	s := newTestStore(t)
	mustCreate(t, s, "Wireless Keyboard", "KB-001", 1, 1)
	mustCreate(t, s, "Wired Mouse", "MS-002", 1, 1)
	mustCreate(t, s, "100% Cotton Mat", "MAT-50%-OFF", 1, 1)
	mustCreate(t, s, "Underscore Item", "AB_CD", 1, 1)

	tests := []struct {
		name   string
		search string
		want   []string
	}{
		{name: "matches name", search: "keyboard", want: []string{"KB-001"}},
		{name: "is case-insensitive", search: "WIRELESS", want: []string{"KB-001"}},
		{name: "matches sku", search: "MS-", want: []string{"MS-002"}},
		{name: "no match returns nothing", search: "zzz", want: nil},
		// Without escaping, these LIKE metacharacters would act as wildcards
		// and match every row instead of the one containing them literally.
		{name: "percent is literal", search: "%", want: []string{"MAT-50%-OFF"}},
		{name: "underscore is literal", search: "_", want: []string{"AB_CD"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			items, total, err := s.List(ListParams{Search: tc.search})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if total != len(tc.want) {
				t.Fatalf("total = %d, want %d (got %+v)", total, len(tc.want), skus(items))
			}
			for i, sku := range tc.want {
				if items[i].SKU != sku {
					t.Errorf("item %d = %s, want %s", i, items[i].SKU, sku)
				}
			}
		})
	}
}

func skus(items []models.Product) []string {
	out := make([]string, len(items))
	for i, p := range items {
		out[i] = p.SKU
	}
	return out
}
