package authz

import (
	"context"

	"github.com/google/uuid"
)

type OpenFGATupleSource interface {
	ListOpenFGATuples(ctx context.Context) ([]OpenFGATuple, error)
}

type OpenFGATupleSyncer struct {
	writer OpenFGATupleWriter
	source OpenFGATupleSource
}

func NewOpenFGATupleSyncer(writer OpenFGATupleWriter, source OpenFGATupleSource) *OpenFGATupleSyncer {
	return &OpenFGATupleSyncer{writer: writer, source: source}
}

func (s *OpenFGATupleSyncer) Backfill(ctx context.Context) error {
	if s == nil || s.writer == nil || s.source == nil {
		return nil
	}
	tuples, err := s.source.ListOpenFGATuples(ctx)
	if err != nil {
		return err
	}
	return s.writer.WriteTuples(ctx, tuples, nil)
}

func (s *OpenFGATupleSyncer) SyncMembership(ctx context.Context, membership Membership) error {
	if s == nil || s.writer == nil {
		return nil
	}
	tuple, ok := OpenFGATupleForMembership(membership)
	if ok {
		return s.writer.WriteTuples(ctx, []OpenFGATuple{tuple}, nil)
	}
	membership.Status = "active"
	tuple, ok = OpenFGATupleForMembership(membership)
	if !ok {
		return nil
	}
	return s.writer.WriteTuples(ctx, nil, []OpenFGATuple{tuple})
}

func (s *OpenFGATupleSyncer) SyncProjectTeamScope(ctx context.Context, tenantID, userID, teamID uuid.UUID, status string) error {
	if s == nil || s.writer == nil {
		return nil
	}
	tuple, ok := OpenFGATupleForProjectTeamScope(tenantID, userID, teamID, status)
	if ok {
		return s.writer.WriteTuples(ctx, []OpenFGATuple{tuple}, nil)
	}
	tuple, ok = OpenFGATupleForProjectTeamScope(tenantID, userID, teamID, "active")
	if !ok {
		return nil
	}
	return s.writer.WriteTuples(ctx, nil, []OpenFGATuple{tuple})
}
