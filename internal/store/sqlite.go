package store

import (
	"database/sql"
	"errors"
	"strings"

	"go-backend/internal/models"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct{ db *sql.DB }

func NewSQLite(path string) (*SQLiteStore, error) {
	// SQLite allows only one writer. Three settings make concurrent writes safe:
	//   busy_timeout   — wait (up to 5s) for the write lock instead of failing with SQLITE_BUSY
	//   journal_mode   — WAL lets readers proceed while a write is in flight
	//   _txlock=immediate — take the write lock at BEGIN, not on first write; this
	//                       avoids the upgrade deadlock (two txns holding a read lock,
	//                       both trying to upgrade) that busy_timeout can't resolve.
	db, err := sql.Open("sqlite",
		path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS products (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			name           TEXT    NOT NULL,
			sku            TEXT    NOT NULL UNIQUE,
			price          REAL    NOT NULL,
			stock_quantity INTEGER NOT NULL DEFAULT 0
		);`)
	return err
}

func (s *SQLiteStore) List(p ListParams) ([]models.Product, int, error) {
	// Build a shared WHERE clause so the count and the page use the same filter.
	where, args := "", []any{}
	if p.Search != "" {
		// LIKE is case-insensitive for ASCII in SQLite; match name OR sku.
		where = ` WHERE name LIKE ? OR sku LIKE ?`
		like := "%" + p.Search + "%"
		args = append(args, like, like)
	}

	// Total count of matching rows (ignores limit/offset) for pagination metadata.
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM products`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Page of rows. Append limit/offset args after the WHERE args.
	q := `SELECT id, name, sku, price, stock_quantity FROM products` + where + ` ORDER BY id LIMIT ? OFFSET ?`
	rows, err := s.db.Query(q, append(args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	list := make([]models.Product, 0)
	for rows.Next() {
		var prod models.Product
		if err := rows.Scan(&prod.ID, &prod.Name, &prod.SKU, &prod.Price, &prod.StockQuantity); err != nil {
			return nil, 0, err
		}
		list = append(list, prod)
	}
	return list, total, rows.Err()
}

func (s *SQLiteStore) Get(id int) (models.Product, error) {
	var p models.Product
	err := s.db.QueryRow(
		`SELECT id, name, sku, price, stock_quantity FROM products WHERE id = ?`, id,
	).Scan(&p.ID, &p.Name, &p.SKU, &p.Price, &p.StockQuantity)

	if errors.Is(err, sql.ErrNoRows) {
		return models.Product{}, ErrNotFound
	}
	if err != nil {
		return models.Product{}, err
	}
	return p, nil
}

func (s *SQLiteStore) Create(p models.Product) (models.Product, error) {
	res, err := s.db.Exec(
		`INSERT INTO products (name, sku, price, stock_quantity) VALUES (?, ?, ?, ?)`,
		p.Name, p.SKU, p.Price, p.StockQuantity,
	)
	if err != nil {
		// Only treat UNIQUE violations as duplicate SKU; surface anything else as-is.
		if strings.Contains(err.Error(), "UNIQUE") {
			return models.Product{}, ErrDuplicateSKU
		}
		return models.Product{}, err
	}
	id, _ := res.LastInsertId()
	p.ID = int(id)
	return p, nil
}

// AdjustStock uses a transaction so read-check-write can't race.
func (s *SQLiteStore) AdjustStock(id, delta int) (models.Product, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return models.Product{}, err
	}
	defer tx.Rollback()

	var current int
	err = tx.QueryRow(`SELECT stock_quantity FROM products WHERE id = ?`, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Product{}, ErrNotFound
	}
	if err != nil {
		return models.Product{}, err
	}

	newQty := current + delta
	if newQty < 0 {
		return models.Product{}, ErrInsufficientStock
	}

	if _, err := tx.Exec(`UPDATE products SET stock_quantity = ? WHERE id = ?`, newQty, id); err != nil {
		return models.Product{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.Product{}, err
	}
	return s.Get(id)
}
