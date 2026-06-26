package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestPgRepositoryDeleteTeamRollsBackWhenSoftDeleteFails(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	unbindCalled := false
	commitCalled := false
	rollbackCalled := false
	deleteErr := errors.New("delete failed")

	tx := &stubTx{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			if sql == queries.UnbindTeamDigitalEmployees {
				unbindCalled = true
				if len(args) != 2 || args[0] != teamID || args[1] != tenantID {
					t.Fatalf("unexpected unbind args: %#v", args)
				}
				return pgconn.NewCommandTag("UPDATE 2"), nil
			}
			t.Fatalf("unexpected exec SQL: %s", sql)
			return pgconn.CommandTag{}, nil
		},
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			if sql == queries.SoftDeleteTeam {
				if len(args) != 2 || args[0] != teamID || args[1] != tenantID {
					t.Fatalf("unexpected soft delete args: %#v", args)
				}
				return stubRow{err: deleteErr}
			}
			t.Fatalf("unexpected query row SQL: %s", sql)
			return stubRow{err: errors.New("unexpected query")}
		},
		commitFn: func(context.Context) error {
			commitCalled = true
			return nil
		},
		rollbackFn: func(context.Context) error {
			rollbackCalled = true
			return nil
		},
	}

	repo := &PgRepository{
		q:  queries.New(nil),
		db: stubBeginner{tx: tx},
	}

	err := repo.DeleteTeam(context.Background(), tenantID, teamID)
	if !errors.Is(err, deleteErr) {
		t.Fatalf("expected delete error to surface, got %v", err)
	}
	if !unbindCalled {
		t.Fatal("expected unbind query to execute before soft delete")
	}
	if commitCalled {
		t.Fatal("expected transaction not to commit on soft delete failure")
	}
	if !rollbackCalled {
		t.Fatal("expected transaction rollback on soft delete failure")
	}
}

type stubBeginner struct {
	tx pgx.Tx
}

func (s stubBeginner) Begin(context.Context) (pgx.Tx, error) {
	return s.tx, nil
}

type stubTx struct {
	execFn     func(context.Context, string, ...any) (pgconn.CommandTag, error)
	queryRowFn func(context.Context, string, ...any) pgx.Row
	commitFn   func(context.Context) error
	rollbackFn func(context.Context) error
}

func (s *stubTx) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("unsupported") }
func (s *stubTx) Commit(ctx context.Context) error {
	if s.commitFn != nil {
		return s.commitFn(ctx)
	}
	return nil
}
func (s *stubTx) Rollback(ctx context.Context) error {
	if s.rollbackFn != nil {
		return s.rollbackFn(ctx)
	}
	return nil
}
func (s *stubTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("unsupported")
}
func (s *stubTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (s *stubTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (s *stubTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("unsupported")
}
func (s *stubTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if s.execFn == nil {
		return pgconn.CommandTag{}, errors.New("unsupported")
	}
	return s.execFn(ctx, sql, args...)
}
func (s *stubTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("unsupported")
}
func (s *stubTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if s.queryRowFn == nil {
		return stubRow{err: errors.New("unsupported")}
	}
	return s.queryRowFn(ctx, sql, args...)
}
func (s *stubTx) Conn() *pgx.Conn { return nil }

type stubRow struct {
	err error
}

func (r stubRow) Scan(...any) error {
	return r.err
}
