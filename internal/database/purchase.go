package database

import (
	"context"
	"errors"
	"fmt"
	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"time"
)

type InvoiceType string

const (
	InvoiceTypeCrypto   InvoiceType = "crypto"
	InvoiceTypeYookasa  InvoiceType = "yookasa"
	InvoiceTypeTelegram InvoiceType = "telegram"
	InvoiceTypeTribute  InvoiceType = "tribute"
)

type PurchaseStatus string

const (
	PurchaseStatusNew     PurchaseStatus = "new"
	PurchaseStatusPending PurchaseStatus = "pending"
	PurchaseStatusPaid    PurchaseStatus = "paid"
	PurchaseStatusCancel  PurchaseStatus = "cancel"
)

type Purchase struct {
	ID                int64          `db:"id"`
	Amount            float64        `db:"amount"`
	CustomerID        int64          `db:"customer_id"`
	CreatedAt         time.Time      `db:"created_at"`
	Month             int            `db:"month"`
	PaidAt            *time.Time     `db:"paid_at"`
	Currency          string         `db:"currency"`
	ExpireAt          *time.Time     `db:"expire_at"`
	Status            PurchaseStatus `db:"status"`
	InvoiceType       InvoiceType    `db:"invoice_type"`
	CryptoInvoiceID   *int64         `db:"crypto_invoice_id"`
	CryptoInvoiceLink *string        `db:"crypto_invoice_url"`
	YookasaURL        *string        `db:"yookasa_url"`
	YookasaID         *uuid.UUID     `db:"yookasa_id"`
}

type PurchaseRepository struct {
	pool *pgxpool.Pool
}

func NewPurchaseRepository(pool *pgxpool.Pool) *PurchaseRepository {
	return &PurchaseRepository{
		pool: pool,
	}
}

func (cr *PurchaseRepository) Create(ctx context.Context, purchase *Purchase) (int64, error) {
	buildInsert := sq.Insert("purchase").
		Columns("amount", "customer_id", "month", "currency", "expire_at", "status", "invoice_type", "crypto_invoice_id", "crypto_invoice_url", "yookasa_url", "yookasa_id").
		Values(purchase.Amount, purchase.CustomerID, purchase.Month, purchase.Currency, purchase.ExpireAt, purchase.Status, purchase.InvoiceType, purchase.CryptoInvoiceID, purchase.CryptoInvoiceLink, purchase.YookasaURL, purchase.YookasaID).
		Suffix("RETURNING id").
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildInsert.ToSql()
	if err != nil {
		return 0, err
	}

	var id int64
	err = cr.pool.QueryRow(ctx, sql, args...).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (cr *PurchaseRepository) FindByInvoiceTypeAndStatus(ctx context.Context, invoiceType InvoiceType, status PurchaseStatus) (*[]Purchase, error) {
	buildSelect := sq.Select("*").
		From("purchase").
		Where(sq.And{
			sq.Eq{"invoice_type": invoiceType},
			sq.Eq{"status": status},
		}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}

	rows, err := cr.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query purchases: %w", err)
	}
	defer rows.Close()

	purchases := []Purchase{}
	for rows.Next() {
		purchase := Purchase{}
		err = rows.Scan(
			&purchase.ID,
			&purchase.Amount,
			&purchase.CustomerID,
			&purchase.CreatedAt,
			&purchase.Month,
			&purchase.PaidAt,
			&purchase.Currency,
			&purchase.ExpireAt,
			&purchase.Status,
			&purchase.InvoiceType,
			&purchase.CryptoInvoiceID,
			&purchase.CryptoInvoiceLink,
			&purchase.YookasaURL,
			&purchase.YookasaID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan purchase: %w", err)
		}
		purchases = append(purchases, purchase)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return &purchases, nil
}

func (cr *PurchaseRepository) FindById(ctx context.Context, id int64) (*Purchase, error) {
	buildSelect := sq.Select("*").
		From("purchase").
		Where(sq.Eq{"id": id}).
		PlaceholderFormat(sq.Dollar)

	sql, args, err := buildSelect.ToSql()
	if err != nil {
		return nil, err
	}
	purchase := &Purchase{}

	err = cr.pool.QueryRow(ctx, sql, args...).Scan(
		&purchase.ID,
		&purchase.Amount,
		&purchase.CustomerID,
		&purchase.CreatedAt,
		&purchase.Month,
		&purchase.PaidAt,
		&purchase.Currency,
		&purchase.ExpireAt,
		&purchase.Status,
		&purchase.InvoiceType,
		&purchase.CryptoInvoiceID,
		&purchase.CryptoInvoiceLink,
		&purchase.YookasaURL,
		&purchase.YookasaID,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query purchase: %w", err)
	}

	return purchase, nil
}

func (p *PurchaseRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	buildUpdate := sq.Update("purchase").
		PlaceholderFormat(sq.Dollar).
		Where(sq.Eq{"id": id})

	for field, value := range updates {
		buildUpdate = buildUpdate.Set(field, value)
	}

	sql, args, err := buildUpdate.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	result, err := p.pool.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no customer found with id: %d", id)
	}

	return nil
}

func (pr *PurchaseRepository) MarkAsPaid(ctx context.Context, purchaseID int64) error {
	currentTime := time.Now()

	updates := map[string]interface{}{
		"status":  PurchaseStatusPaid,
		"paid_at": currentTime,
	}

	return pr.UpdateFields(ctx, purchaseID, updates)
}
