package project

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// enrichMemberDisplayNames overlays current user/employee names onto member
// display_name_snapshot for read models (D4=A). Missing principals keep the
// stored snapshot; still-empty names stay empty for the client to fall back to id.
func (s *Service) enrichMemberDisplayNames(ctx context.Context, tenantID uuid.UUID, members []ProjectMember) []ProjectMember {
	if len(members) == 0 {
		return members
	}
	enricher, ok := s.repository.(memberDisplayNameEnricher)
	if !ok {
		return members
	}
	userIDs := make([]uuid.UUID, 0, len(members))
	employeeIDs := make([]uuid.UUID, 0, len(members))
	seenUsers := map[uuid.UUID]struct{}{}
	seenEmployees := map[uuid.UUID]struct{}{}
	for _, member := range members {
		switch member.PrincipalType {
		case PrincipalTypeHumanUser:
			if member.PrincipalID == uuid.Nil {
				continue
			}
			if _, ok := seenUsers[member.PrincipalID]; ok {
				continue
			}
			seenUsers[member.PrincipalID] = struct{}{}
			userIDs = append(userIDs, member.PrincipalID)
		case PrincipalTypeDigitalEmployee:
			if member.PrincipalID == uuid.Nil {
				continue
			}
			if _, ok := seenEmployees[member.PrincipalID]; ok {
				continue
			}
			seenEmployees[member.PrincipalID] = struct{}{}
			employeeIDs = append(employeeIDs, member.PrincipalID)
		}
	}
	userNames, employeeNames, err := enricher.LookupMemberDisplayNames(ctx, tenantID, userIDs, employeeIDs)
	if err != nil {
		return members
	}
	out := make([]ProjectMember, len(members))
	copy(out, members)
	for i := range out {
		var current string
		switch out[i].PrincipalType {
		case PrincipalTypeHumanUser:
			current = userNames[out[i].PrincipalID]
		case PrincipalTypeDigitalEmployee:
			current = employeeNames[out[i].PrincipalID]
		}
		current = strings.TrimSpace(current)
		if current == "" {
			continue
		}
		name := current
		out[i].DisplayNameSnapshot = &name
	}
	return out
}

type memberDisplayNameEnricher interface {
	LookupMemberDisplayNames(ctx context.Context, tenantID uuid.UUID, userIDs, employeeIDs []uuid.UUID) (map[uuid.UUID]string, map[uuid.UUID]string, error)
}
