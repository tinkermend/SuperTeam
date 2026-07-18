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
			switch sql {
			case queries.UnbindTeamDigitalEmployees:
				unbindCalled = true
				if len(args) != 2 || args[0] != teamID || args[1] != tenantID {
					t.Fatalf("unexpected unbind args: %#v", args)
				}
				return pgconn.NewCommandTag("UPDATE 2"), nil
			case queries.DeleteTeamSkillBindings, queries.SoftDeleteTeamMCPBindings:
				if len(args) != 2 || args[0] != tenantID || args[1] != teamID {
					t.Fatalf("unexpected binding cleanup args: %#v", args)
				}
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}
			t.Fatalf("unexpected exec SQL: %s", sql)
			return pgconn.CommandTag{}, nil
		},
		queryRowFn: func(_ context.Context, sql string, args ...any) pgx.Row {
			if sql == queries.SoftDeleteTeam {
				// P2:软删带删除发起人(args[0]=delete_requested_by)。
				if len(args) != 3 || args[1] != teamID || args[2] != tenantID {
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

	err := repo.DeleteTeam(context.Background(), tenantID, teamID, uuid.New())
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

func TestPgRepositoryDeleteTeamCleansBindingsBeforeSoftDelete(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	var execOrder []string
	commitCalled := false

	tx := &stubTx{
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			switch sql {
			case queries.UnbindTeamDigitalEmployees:
				execOrder = append(execOrder, "unbind")
				return pgconn.NewCommandTag("UPDATE 1"), nil
			case queries.DeleteTeamSkillBindings:
				if len(args) != 2 || args[0] != tenantID || args[1] != teamID {
					t.Fatalf("unexpected skill binding cleanup args: %#v", args)
				}
				execOrder = append(execOrder, "skill")
				return pgconn.NewCommandTag("DELETE 3"), nil
			case queries.SoftDeleteTeamMCPBindings:
				if len(args) != 2 || args[0] != tenantID || args[1] != teamID {
					t.Fatalf("unexpected mcp binding cleanup args: %#v", args)
				}
				execOrder = append(execOrder, "mcp")
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			t.Fatalf("unexpected exec SQL: %s", sql)
			return pgconn.CommandTag{}, nil
		},
		queryRowFn: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if sql != queries.SoftDeleteTeam {
				t.Fatalf("unexpected query row SQL: %s", sql)
			}
			execOrder = append(execOrder, "soft-delete")
			return stubRow{err: pgx.ErrNoRows}
		},
		commitFn: func(context.Context) error {
			commitCalled = true
			return nil
		},
	}

	repo := &PgRepository{
		q:  queries.New(nil),
		db: stubBeginner{tx: tx},
	}

	err := repo.DeleteTeam(context.Background(), tenantID, teamID, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found from soft delete no rows, got %v", err)
	}
	want := []string{"unbind", "skill", "mcp", "soft-delete"}
	if len(execOrder) != len(want) {
		t.Fatalf("unexpected exec order: %v", execOrder)
	}
	for i, step := range want {
		if execOrder[i] != step {
			t.Fatalf("unexpected exec order: %v", execOrder)
		}
	}
	if commitCalled {
		t.Fatal("expected no commit when soft delete finds no rows")
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
