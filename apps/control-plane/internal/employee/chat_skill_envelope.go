package employee

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/skill"
)

// applyChatSkillEnvelope narrows three-layer supply to the Chat default
// surface (project skill bindings) and optional per-turn selection (autonomy P1).
//
// Rules:
//   - allowIDs = project skill binding skill_ids (Chat default surface)
//   - empty selected → keep supply ∩ allowIDs
//   - non-empty selected → each id must be in allowIDs; result = supply ∩ selected
//   - skills in supply but not in allowIDs are dropped (employee-only skills
//     outside project bindings do not enter Chat)
func applyChatSkillEnvelope(
	supply []skill.SkillRuntimeRecord,
	allowIDs map[uuid.UUID]struct{},
	selected []uuid.UUID,
) ([]skill.SkillRuntimeRecord, error) {
	if len(selected) > 0 {
		for _, id := range selected {
			if _, ok := allowIDs[id]; !ok {
				return nil, fmt.Errorf("%w: skill_id %s is outside the project Chat skill surface", ErrInvalidInput, id)
			}
		}
		want := make(map[uuid.UUID]struct{}, len(selected))
		for _, id := range selected {
			want[id] = struct{}{}
		}
		out := make([]skill.SkillRuntimeRecord, 0, len(selected))
		for _, rec := range supply {
			if _, ok := want[rec.ID]; ok {
				out = append(out, rec)
			}
		}
		// Selected ids that are in the allow surface but not in supply still
		// count as accepted declarations; projection simply has nothing to
		// materialize for them (e.g. archived skill still bound). Callers may
		// still audit selected ids via metadata.
		return out, nil
	}

	if len(allowIDs) == 0 {
		// No project bindings → Chat projects no skills (strict default surface).
		return nil, nil
	}
	out := make([]skill.SkillRuntimeRecord, 0, len(supply))
	for _, rec := range supply {
		if _, ok := allowIDs[rec.ID]; ok {
			out = append(out, rec)
		}
	}
	return out, nil
}

func skillIDSet(ids []uuid.UUID) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		out[id] = struct{}{}
	}
	return out
}
