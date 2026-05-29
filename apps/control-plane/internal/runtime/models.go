package runtime

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// NodeStatus represents the status of a runtime node
type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
)

// IsValid checks if the status is valid
func (s NodeStatus) IsValid() bool {
	switch s {
	case NodeStatusOnline, NodeStatusOffline:
		return true
	}
	return false
}

// Node represents a runtime node in the domain model
type Node struct {
	ID                 int64
	NodeID             string
	Name               string
	SupportedProviders []string
	MaxSlots           int32
	CurrentLoad        int32
	Status             NodeStatus
	Metadata           map[string]interface{}
	LastHeartbeatAt    time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// IsOnline checks if the node is online based on heartbeat
// A node is considered online if it has sent a heartbeat within the last 60 seconds
func (n *Node) IsOnline() bool {
	return time.Since(n.LastHeartbeatAt) <= 60*time.Second
}

// HasCapacity checks if the node has available slots
func (n *Node) HasCapacity() bool {
	return n.CurrentLoad < n.MaxSlots
}

// SupportsProvider checks if the node supports a given provider type
func (n *Node) SupportsProvider(providerType string) bool {
	for _, p := range n.SupportedProviders {
		if p == providerType {
			return true
		}
	}
	return false
}

// RegisterNodeRequest represents a request to register a runtime node
type RegisterNodeRequest struct {
	NodeID             string
	Name               string
	SupportedProviders []string
	MaxSlots           int32
	Metadata           map[string]interface{}
}

// UpdateHeartbeatRequest represents a request to update node heartbeat
type UpdateHeartbeatRequest struct {
	NodeID      string
	CurrentLoad int32
}

// ListNodesFilter represents filters for listing nodes
type ListNodesFilter struct {
	Status *NodeStatus
	Limit  int32
	Offset int32
}

// Helper functions to convert between pgtype and domain types

func textFromString(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func stringFromText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func int8FromInt64(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{Valid: false}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func int64FromInt8(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}

func timeFromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
