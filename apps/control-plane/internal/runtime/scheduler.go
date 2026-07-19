package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/superteam/control-plane/internal/platform"
)

var (
	ErrNoAvailableNode = errors.New("no available node found")
)

// Scheduler handles task scheduling to runtime nodes
type Scheduler struct {
	repository               Repository
	heartbeatTimeoutResolver HeartbeatTimeoutResolver
}

// NewScheduler creates a new scheduler instance
func NewScheduler(repository Repository) (*Scheduler, error) {
	if repository == nil {
		return nil, errors.New("runtime repository is required")
	}
	return &Scheduler{
		repository: repository,
	}, nil
}

// SetHeartbeatTimeoutResolver 注入心跳超时解析闭包(app 装配层接线,
// 与 Service 同法);未注入回退默认常量。
func (s *Scheduler) SetHeartbeatTimeoutResolver(resolver HeartbeatTimeoutResolver) {
	s.heartbeatTimeoutResolver = resolver
}

// heartbeatTimeout 解析调度用的心跳超时。SelectNode 是跨租户候选查询,
// 无租户轴,按默认租户解析(节点表当前为单默认租户)。
func (s *Scheduler) heartbeatTimeout(ctx context.Context) time.Duration {
	if s.heartbeatTimeoutResolver == nil {
		return HeartbeatTimeout
	}
	return s.heartbeatTimeoutResolver(ctx, platform.DefaultTenantID)
}

// SelectNode selects the best available node for a given provider type.
//
// Concurrency model: the candidate list is read without a lock and is only used
// to decide the order in which nodes are tried (lowest current_load first).
// The actual slot reservation happens atomically inside TryAcquireNodeSlot,
// whose SQL UPDATE inlines the capacity guard (current_load < max_slots) and
// liveness guards (status = 'online', fresh heartbeat, not archived/disabled).
// PostgreSQL serializes concurrent UPDATEs on the same row and re-evaluates the
// WHERE clause against the latest committed version, so two concurrent
// SelectNode calls cannot both reserve the last slot on the same node: the
// second one receives pgx.ErrNoRows and falls through to the next candidate.
func (s *Scheduler) SelectNode(ctx context.Context, providerType string) (*Node, error) {
	if providerType == "" {
		return nil, errors.New("provider_type is required")
	}

	// Get all online nodes (used only to order candidates; no lock taken here)
	threshold := time.Now().Add(-s.heartbeatTimeout(ctx))
	thresholdTs := timestamptzFromTime(threshold)
	nodes, err := s.repository.ListOnlineNodes(ctx, thresholdTs)
	if err != nil {
		return nil, fmt.Errorf("failed to list online nodes: %w", err)
	}

	// Filter nodes that support the provider; capacity is enforced atomically
	// at acquire time, so we don't pre-filter by HasCapacity here.
	var candidates []*Node
	for _, record := range nodes {
		node, err := s.recordToNode(ctx, record)
		if err != nil {
			continue
		}
		if !node.SupportsProvider(providerType) {
			continue
		}
		candidates = append(candidates, node)
	}

	if len(candidates) == 0 {
		return nil, ErrNoAvailableNode
	}

	// Order candidates by current load (ascending) so the least-loaded viable
	// node is tried first. Ties keep the stable DB ordering (already by
	// current_load ASC, created_at ASC).
	sortCandidatesByLoad(candidates)

	// Try each candidate atomically. The first success wins; a pgx.ErrNoRows
	// result means the node was full (or went offline/stale) between the read
	// and the acquire, so move on to the next candidate.
	for _, node := range candidates {
		record, err := s.repository.TryAcquireNodeSlot(ctx, node.NodeID, thresholdTs)
		if err == nil {
			return s.recordToNode(ctx, record)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		return nil, fmt.Errorf("failed to acquire node slot: %w", err)
	}

	return nil, ErrNoAvailableNode
}

// sortCandidatesByLoad orders nodes by current load ascending, preserving the
// input order for ties (which already comes from the DB ordered by
// current_load ASC, created_at ASC).
func sortCandidatesByLoad(candidates []*Node) {
	// Simple insertion sort: candidate lists are small (number of online
	// runtime nodes per provider) and stability is required to keep the DB's
	// tiebreaker ordering.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && candidates[j].CurrentLoad < candidates[j-1].CurrentLoad; j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}
}

// Helper method to convert record to node
func (s *Scheduler) recordToNode(ctx context.Context, record NodeRecord) (*Node, error) {
	// Use the same conversion logic as Service, carrying the resolver so the
	// derived-status freshness check uses the configured timeout.
	svc := &Service{repository: s.repository, heartbeatTimeoutResolver: s.heartbeatTimeoutResolver}
	return svc.recordToNode(ctx, record)
}
