package authz

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type recordingTupleWriter struct {
	writes  []OpenFGATuple
	deletes []OpenFGATuple
}

func (w *recordingTupleWriter) WriteTuples(ctx context.Context, writes, deletes []OpenFGATuple) error {
	w.writes = append(w.writes, writes...)
	w.deletes = append(w.deletes, deletes...)
	return nil
}

type staticTupleSource struct {
	tuples []OpenFGATuple
}

func (s staticTupleSource) ListOpenFGATuples(ctx context.Context) ([]OpenFGATuple, error) {
	return s.tuples, nil
}

func TestOpenFGATupleSyncerWritesActiveAndDeletesRevokedProjectScope(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	teamID := uuid.MustParse("00000000-0000-4000-8000-000000000010")
	writer := &recordingTupleWriter{}
	syncer := NewOpenFGATupleSyncer(writer, nil)

	require.NoError(t, syncer.SyncProjectTeamScope(context.Background(), tenantID, userID, teamID, "active"))
	require.NoError(t, syncer.SyncProjectTeamScope(context.Background(), tenantID, userID, teamID, "revoked"))

	require.Len(t, writer.writes, 1)
	require.Len(t, writer.deletes, 1)
	require.Equal(t, writer.writes[0], writer.deletes[0])
	require.Equal(t, OpenFGARelationProjectScopeUser, writer.writes[0].Relation)
}

func TestOpenFGATupleSyncerBackfillsSourceTuples(t *testing.T) {
	writer := &recordingTupleWriter{}
	source := staticTupleSource{tuples: []OpenFGATuple{
		{User: "user:alice", Relation: "admin", Object: "tenant:t1"},
		{User: "user:alice", Relation: "project_scope_user", Object: "team:team1"},
	}}
	syncer := NewOpenFGATupleSyncer(writer, source)

	require.NoError(t, syncer.Backfill(context.Background()))

	require.Equal(t, source.tuples, writer.writes)
	require.Empty(t, writer.deletes)
}
