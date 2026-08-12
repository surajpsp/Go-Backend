package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"go-backend/internal/models"
	"go-backend/internal/response"
	"go-backend/internal/router"
	"go-backend/internal/store"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode) // silence gin's debug route dump
	m.Run()
}

// newAPI returns the real router — same middleware chain and routes as main —
// wired to a throwaway SQLite file, plus the store for arranging fixtures.
func newAPI(t *testing.T) (*gin.Engine, *store.SQLiteStore) {
	t.Helper()
	s, err := store.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return router.New(s), s
}

// do issues a request against the engine and returns the recorded response.
func do(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// decode unmarshals the envelope, failing the test if the body is not one.
func decode(t *testing.T, w *httptest.ResponseRecorder) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not a JSON envelope: %v\nbody: %s", err, w.Body.String())
	}
	if env.Code != w.Code {
		t.Errorf("envelope code %d does not match HTTP status %d", env.Code, w.Code)
	}
	return env
}

// productFrom pulls the product out of an envelope's data field.
func productFrom(t *testing.T, env response.Envelope) models.Product {
	t.Helper()
	raw, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("re-marshal data: %v", err)
	}
	var p models.Product
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("data is not a product: %v", err)
	}
	return p
}

func seedProduct(t *testing.T, s *store.SQLiteStore, sku string, stock int) models.Product {
	t.Helper()
	p, err := s.Create(models.Product{Name: "Item " + sku, SKU: sku, Price: 9.99, StockQuantity: stock})
	if err != nil {
		t.Fatalf("seed %s: %v", sku, err)
	}
	return p
}

func TestListEmpty(t *testing.T) {
	r, _ := newAPI(t)

	w := do(t, r, http.MethodGet, "/products", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	// The client renders an empty state from [], and would crash on null.
	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Errorf("empty list must serialise as [], got: %s", w.Body.String())
	}
}

// GET /products with no query params returns every product, per the API
// contract. Pagination is opt-in and must not silently truncate the default.
func TestListReturnsAllByDefault(t *testing.T) {
	r, s := newAPI(t)
	const n = 25
	for i := range n {
		seedProduct(t, s, "SKU-"+string(rune('A'+i)), 1)
	}

	env := decode(t, do(t, r, http.MethodGet, "/products", ""))
	items, ok := env.Data.([]any)
	if !ok {
		t.Fatalf("data is not a list: %T", env.Data)
	}
	if len(items) != n {
		t.Errorf("returned %d products, want all %d", len(items), n)
	}
	if env.Pagination == nil || env.Pagination.Total != n {
		t.Errorf("pagination = %+v, want total %d", env.Pagination, n)
	}
}

func TestListPagination(t *testing.T) {
	r, s := newAPI(t)
	for i := range 5 {
		seedProduct(t, s, "SKU-"+string(rune('A'+i)), 1)
	}

	env := decode(t, do(t, r, http.MethodGet, "/products?page=2&limit=2", ""))
	items := env.Data.([]any)
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}
	want := response.Pagination{Page: 2, Limit: 2, Total: 5, TotalPages: 3}
	if *env.Pagination != want {
		t.Errorf("pagination = %+v, want %+v", *env.Pagination, want)
	}
}

func TestListRejectsBadPaginationParams(t *testing.T) {
	r, _ := newAPI(t)

	for _, q := range []string{"?page=0", "?page=abc", "?limit=0", "?limit=-3", "?limit=x"} {
		t.Run(q, func(t *testing.T) {
			w := do(t, r, http.MethodGet, "/products"+q, "")
			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestGetProduct(t *testing.T) {
	r, s := newAPI(t)
	created := seedProduct(t, s, "KB-001", 10)

	env := decode(t, do(t, r, http.MethodGet, "/products/"+strconv.Itoa(created.ID), ""))
	if got := productFrom(t, env); got != created {
		t.Errorf("got %+v, want %+v", got, created)
	}
}

func TestGetProductErrors(t *testing.T) {
	r, _ := newAPI(t)

	tests := []struct {
		name, path, wantErr string
		wantStatus          int
	}{
		{name: "missing id", path: "/products/999", wantStatus: http.StatusNotFound, wantErr: "not_found"},
		{name: "non-numeric id", path: "/products/abc", wantStatus: http.StatusBadRequest, wantErr: "invalid_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, r, http.MethodGet, tc.path, "")
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if env := decode(t, w); env.Error != tc.wantErr {
				t.Errorf("error = %q, want %q", env.Error, tc.wantErr)
			}
		})
	}
}

func TestCreateProduct(t *testing.T) {
	r, _ := newAPI(t)

	w := do(t, r, http.MethodPost, "/products",
		`{"name":"Wireless Keyboard","sku":"KB-001","price":35.50,"stockQuantity":10}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
	}

	got := productFrom(t, decode(t, w))
	want := models.Product{ID: got.ID, Name: "Wireless Keyboard", SKU: "KB-001", Price: 35.50, StockQuantity: 10}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.ID == 0 {
		t.Error("created product has no id")
	}
}

func TestCreateProductTrimsWhitespace(t *testing.T) {
	r, _ := newAPI(t)

	env := decode(t, do(t, r, http.MethodPost, "/products",
		`{"name":"  Padded Name  ","sku":"  SKU-1  ","price":1,"stockQuantity":0}`))
	got := productFrom(t, env)
	if got.Name != "Padded Name" || got.SKU != "SKU-1" {
		t.Errorf("stored name=%q sku=%q, want them trimmed", got.Name, got.SKU)
	}
}

func TestCreateProductValidation(t *testing.T) {
	tests := []struct {
		name, body  string
		wantStatus  int
		wantErr     string
		wantMessage string
	}{{
		name:        "empty name",
		body:        `{"name":"","sku":"S-1","price":1,"stockQuantity":0}`,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "name is required",
	}, {
		// "   " is non-empty as far as the stock `required` rule is concerned,
		// so this is the case the notblank rule exists for.
		name:        "whitespace-only name",
		body:        `{"name":"   ","sku":"S-1","price":1,"stockQuantity":0}`,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "name is required",
	}, {
		name:        "whitespace-only sku",
		body:        `{"name":"Item","sku":"  ","price":1,"stockQuantity":0}`,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "sku is required",
	}, {
		// The message must state the real rule, not "price is required".
		name:        "zero price",
		body:        `{"name":"Item","sku":"S-1","price":0,"stockQuantity":0}`,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "price must be greater than 0",
	}, {
		name:        "negative price",
		body:        `{"name":"Item","sku":"S-1","price":-1,"stockQuantity":0}`,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "price must be greater than 0",
	}, {
		name:        "negative stock",
		body:        `{"name":"Item","sku":"S-1","price":1,"stockQuantity":-1}`,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "stockQuantity must be at least 0",
	}, {
		name:        "empty body",
		body:        ``,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "request body must be valid JSON",
	}, {
		name:        "malformed json",
		body:        `{"name":`,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "request body must be valid JSON",
	}, {
		name:        "wrong field type",
		body:        `{"name":"Item","sku":"S-1","price":"free","stockQuantity":0}`,
		wantStatus:  http.StatusBadRequest,
		wantErr:     "validation_failed",
		wantMessage: "price has the wrong type",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newAPI(t)
			w := do(t, r, http.MethodPost, "/products", tc.body)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			env := decode(t, w)
			if env.Error != tc.wantErr {
				t.Errorf("error = %q, want %q", env.Error, tc.wantErr)
			}
			if !strings.Contains(env.Message, tc.wantMessage) {
				t.Errorf("message = %q, want it to contain %q", env.Message, tc.wantMessage)
			}
			if env.Data != nil {
				t.Errorf("data = %v, want null on an error", env.Data)
			}
		})
	}
}

// Validation messages must name the JSON fields the client sent, not the Go
// struct fields, or the app cannot map an error back to its form input.
func TestValidationMessagesUseJSONFieldNames(t *testing.T) {
	r, _ := newAPI(t)

	env := decode(t, do(t, r, http.MethodPost, "/products", `{}`))
	for _, goName := range []string{"Name", "SKU", "Price", "StockQuantity"} {
		if strings.Contains(env.Message, goName) {
			t.Errorf("message leaks the Go field name %q: %s", goName, env.Message)
		}
	}
	for _, jsonName := range []string{"name", "sku", "price"} {
		if !strings.Contains(env.Message, jsonName) {
			t.Errorf("message is missing the json field %q: %s", jsonName, env.Message)
		}
	}
}

func TestCreateProductDuplicateSKU(t *testing.T) {
	r, s := newAPI(t)
	seedProduct(t, s, "KB-001", 1)

	w := do(t, r, http.MethodPost, "/products", `{"name":"Other","sku":"KB-001","price":1,"stockQuantity":1}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if env := decode(t, w); env.Error != "duplicate_sku" {
		t.Errorf("error = %q, want duplicate_sku", env.Error)
	}
}

func TestAdjustStock(t *testing.T) {
	tests := []struct {
		name       string
		start      int
		body       string
		wantStatus int
		wantStock  int
		wantErr    string
	}{
		{name: "adds", start: 10, body: `{"quantity":5}`, wantStatus: http.StatusOK, wantStock: 15},
		{name: "removes", start: 10, body: `{"quantity":-2}`, wantStatus: http.StatusOK, wantStock: 8},
		{name: "down to zero", start: 10, body: `{"quantity":-10}`, wantStatus: http.StatusOK, wantStock: 0},
		// A plain int field with `required` rejects 0 as "missing"; the pointer
		// in the request type is what lets an explicit zero through.
		{name: "explicit zero is a no-op", start: 10, body: `{"quantity":0}`, wantStatus: http.StatusOK, wantStock: 10},
		{
			name: "cannot go negative", start: 10, body: `{"quantity":-11}`,
			wantStatus: http.StatusConflict, wantErr: "insufficient_stock",
		},
		{
			name: "missing quantity is rejected", start: 10, body: `{}`,
			wantStatus: http.StatusBadRequest, wantErr: "validation_failed",
		},
		{
			name: "wrong quantity type is rejected", start: 10, body: `{"quantity":"two"}`,
			wantStatus: http.StatusBadRequest, wantErr: "validation_failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, s := newAPI(t)
			p := seedProduct(t, s, "KB-001", tc.start)

			w := do(t, r, http.MethodPost, "/products/"+strconv.Itoa(p.ID)+"/stock", tc.body)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			env := decode(t, w)

			if tc.wantErr != "" {
				if env.Error != tc.wantErr {
					t.Errorf("error = %q, want %q", env.Error, tc.wantErr)
				}
				// A rejected adjustment must leave the stock untouched.
				after, _ := s.Get(p.ID)
				if after.StockQuantity != tc.start {
					t.Errorf("stock changed on a rejected adjustment: %d, want %d", after.StockQuantity, tc.start)
				}
				return
			}

			// The app displays the quantity from this response, so it has to be
			// both correct and consistent with what was stored.
			got := productFrom(t, env)
			if got.StockQuantity != tc.wantStock {
				t.Errorf("returned stock %d, want %d", got.StockQuantity, tc.wantStock)
			}
			after, err := s.Get(p.ID)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got != after {
				t.Errorf("response %+v disagrees with stored %+v", got, after)
			}
		})
	}
}

func TestAdjustStockMissingProduct(t *testing.T) {
	r, _ := newAPI(t)

	w := do(t, r, http.MethodPost, "/products/999/stock", `{"quantity":1}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if env := decode(t, w); env.Error != "not_found" {
		t.Errorf("error = %q, want not_found", env.Error)
	}
}

// Unknown routes and wrong methods must return the same JSON envelope as every
// other failure, so the client has a single parse path.
func TestFrameworkFailuresUseTheEnvelope(t *testing.T) {
	r, _ := newAPI(t)

	tests := []struct {
		name, method, path string
		wantStatus         int
		wantErr            string
	}{
		{name: "unknown route", method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound, wantErr: "route_not_found"},
		{name: "wrong method", method: http.MethodDelete, path: "/products", wantStatus: http.StatusMethodNotAllowed, wantErr: "method_not_allowed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, r, tc.method, tc.path, "")
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if env := decode(t, w); env.Error != tc.wantErr {
				t.Errorf("error = %q, want %q", env.Error, tc.wantErr)
			}
		})
	}
}

// A store failure must surface as a 500 envelope with a generic message — the
// underlying cause belongs in the log, not in the response.
func TestStoreFailureIsNotLeakedToTheClient(t *testing.T) {
	secret := errors.New("connection to db-prod-01 refused: password=hunter2")
	r := router.New(failingStore{err: secret})

	tests := []struct{ name, method, path, body string }{
		{"list", http.MethodGet, "/products", ""},
		{"get", http.MethodGet, "/products/1", ""},
		{"create", http.MethodPost, "/products", `{"name":"Item","sku":"S-1","price":1,"stockQuantity":0}`},
		{"adjust", http.MethodPost, "/products/1/stock", `{"quantity":1}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := do(t, r, tc.method, tc.path, tc.body)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500: %s", w.Code, w.Body.String())
			}
			env := decode(t, w)
			if env.Error != "internal_error" {
				t.Errorf("error = %q, want internal_error", env.Error)
			}
			if strings.Contains(w.Body.String(), "hunter2") {
				t.Errorf("response leaked the underlying error: %s", w.Body.String())
			}
		})
	}
}

// A panic must become a 500 envelope rather than an empty body or a stack dump.
func TestPanicBecomesAnEnvelope(t *testing.T) {
	r := router.New(panicStore{})

	w := do(t, r, http.MethodGet, "/products", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	env := decode(t, w)
	if env.Error != "internal_error" {
		t.Errorf("error = %q, want internal_error", env.Error)
	}
	if strings.Contains(w.Body.String(), "boom") {
		t.Errorf("response leaked the panic value: %s", w.Body.String())
	}
}

func TestHealth(t *testing.T) {
	r, _ := newAPI(t)

	w := do(t, r, http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if env := decode(t, w); env.Status != response.StatusSuccess {
		t.Errorf("status = %q, want success", env.Status)
	}
}

// Every response carries a correlation id in the header; error bodies repeat it
// so a user-reported id maps to a log line.
func TestRequestIDIsEchoedAndCorrelated(t *testing.T) {
	r, _ := newAPI(t)

	w := do(t, r, http.MethodGet, "/products/999", "")
	header := w.Header().Get("X-Request-ID")
	if header == "" {
		t.Fatal("X-Request-ID header is missing")
	}
	if env := decode(t, w); env.RequestID != header {
		t.Errorf("body requestId %q does not match header %q", env.RequestID, header)
	}
}

func TestRequestIDRejectsUntrustedInboundValue(t *testing.T) {
	r, _ := newAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "bad value\r\nX-Injected: yes")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); strings.Contains(got, " ") || strings.Contains(got, "\n") {
		t.Errorf("untrusted request id was echoed back: %q", got)
	}
}

// --- test doubles -----------------------------------------------------------

// failingStore fails every operation with a fixed error.
type failingStore struct{ err error }

func (f failingStore) List(store.ListParams) ([]models.Product, int, error) { return nil, 0, f.err }
func (f failingStore) Get(int) (models.Product, error)                      { return models.Product{}, f.err }
func (f failingStore) Create(models.Product) (models.Product, error)        { return models.Product{}, f.err }
func (f failingStore) AdjustStock(int, int) (models.Product, error)         { return models.Product{}, f.err }

// panicStore panics on List, to exercise the recovery middleware.
type panicStore struct{ failingStore }

func (panicStore) List(store.ListParams) ([]models.Product, int, error) { panic("boom") }
