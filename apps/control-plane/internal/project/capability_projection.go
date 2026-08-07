package project

import (
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/capabilityprojection"
)

// Type aliases keep project call sites and handler mapping stable.
type CapabilityProjectionSnapshot = capabilityprojection.CapabilityProjectionSnapshot
type CapabilityProjectionSummary = capabilityprojection.CapabilityProjectionSummary
type ProjectedSkillItem = capabilityprojection.ProjectedSkillItem
type ProjectedMcpItem = capabilityprojection.ProjectedMcpItem
type ProjectedSkillConflict = capabilityprojection.ProjectedSkillConflict
type CapabilityProjectionSourceRow = capabilityprojection.CapabilityProjectionSourceRow

func emptyCapabilityProjection() CapabilityProjectionSnapshot {
	return capabilityprojection.Empty()
}

func ExtractCapabilityProjection(payloadJSON []byte, attestationConflictJSONList [][]byte) CapabilityProjectionSnapshot {
	return capabilityprojection.Extract(payloadJSON, attestationConflictJSONList)
}

func EnrichCapabilityProjectionNames(snap *CapabilityProjectionSnapshot, namesByID map[uuid.UUID]string) {
	capabilityprojection.EnrichNames(snap, namesByID)
}

func CollectSkillIDsFromProjection(snap CapabilityProjectionSnapshot) []uuid.UUID {
	return capabilityprojection.CollectSkillIDs(snap)
}
