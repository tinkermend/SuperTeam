package handlers

import (
	"encoding/json"

	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
)

type taskResponse struct {
	ID             int64           `json:"id"`
	Title          string          `json:"title"`
	Description    *string         `json:"description,omitempty"`
	CreatorID      *int64          `json:"creator_id,omitempty"`
	ProviderType   string          `json:"provider_type"`
	TargetNodeID   *string         `json:"target_node_id,omitempty"`
	AssignedNodeID *string         `json:"assigned_node_id,omitempty"`
	Status         task.TaskStatus `json:"status"`
	WorkspacePath  *string         `json:"workspace_path,omitempty"`
	Params         json.RawMessage `json:"params"`
	Priority       int32           `json:"priority"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

type runtimeNodeResponse struct {
	NodeID             string                 `json:"node_id"`
	Name               string                 `json:"name"`
	SupportedProviders []string               `json:"supported_providers"`
	MaxSlots           int32                  `json:"max_slots"`
	CurrentLoad        int32                  `json:"current_load"`
	Status             runtime.NodeStatus     `json:"status"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	LastHeartbeatAt    string                 `json:"last_heartbeat_at,omitempty"`
	CreatedAt          string                 `json:"created_at,omitempty"`
	UpdatedAt          string                 `json:"updated_at,omitempty"`
}

func newTaskResponse(t *task.Task) taskResponse {
	return taskResponse{
		ID:             t.ID,
		Title:          t.Title,
		Description:    t.Description,
		CreatorID:      t.CreatorID,
		ProviderType:   t.ProviderType,
		TargetNodeID:   t.TargetNodeID,
		AssignedNodeID: t.AssignedNodeID,
		Status:         t.Status,
		WorkspacePath:  t.WorkspacePath,
		Params:         normalizeTaskParams(t.Params),
		Priority:       t.Priority,
		CreatedAt:      t.CreatedAt.UTC().Format(timeRFC3339Nano),
		UpdatedAt:      t.UpdatedAt.UTC().Format(timeRFC3339Nano),
	}
}

func newTaskResponses(tasks []*task.Task) []taskResponse {
	responses := make([]taskResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, newTaskResponse(t))
	}
	return responses
}

func normalizeTaskParams(raw []byte) json.RawMessage {
	trimmed := json.RawMessage(`{}`)
	if len(raw) == 0 {
		return trimmed
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return trimmed
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return trimmed
	}

	normalized, err := json.Marshal(object)
	if err != nil {
		return trimmed
	}
	return json.RawMessage(normalized)
}

func newRuntimeNodeResponse(node *runtime.Node) runtimeNodeResponse {
	response := runtimeNodeResponse{
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: append([]string(nil), node.SupportedProviders...),
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
	}
	if !node.LastHeartbeatAt.IsZero() {
		response.LastHeartbeatAt = node.LastHeartbeatAt.UTC().Format(timeRFC3339Nano)
	}
	if !node.CreatedAt.IsZero() {
		response.CreatedAt = node.CreatedAt.UTC().Format(timeRFC3339Nano)
	}
	if !node.UpdatedAt.IsZero() {
		response.UpdatedAt = node.UpdatedAt.UTC().Format(timeRFC3339Nano)
	}
	return response
}

func newRuntimeNodeResponses(nodes []*runtime.Node) []runtimeNodeResponse {
	responses := make([]runtimeNodeResponse, 0, len(nodes))
	for _, node := range nodes {
		responses = append(responses, newRuntimeNodeResponse(node))
	}
	return responses
}

const timeRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
