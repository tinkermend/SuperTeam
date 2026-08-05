package scenariotemplate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// RoleHolderCounter returns how many ready/active employees hold a role_key.
// Optional on Service; when nil, holder_count is 0.
type RoleHolderCounter interface {
	CountActiveHolders(ctx context.Context, tenantID uuid.UUID, roleKey string) (int, error)
}

// RoleViewRole is one playbook role with tenant-wide holder count.
type RoleViewRole struct {
	RoleKey              string
	Title                string
	RequiredCapabilities []string
	HolderCount          int
}

// RoleIndependencePair marks two roles that must be distinct people at an exit.
type RoleIndependencePair struct {
	Roles []string
}

// RoleViewExit is one exit deliverable with required roles (server-computed).
type RoleViewExit struct {
	Deliverable            string
	Label                  string
	RequiredRoles          []string
	RoleIndependencePairs  []RoleIndependencePair
}

// RoleView is the read-only "roles + exits" projection for a scenario template.
type RoleView struct {
	TemplateKey string
	Name        string
	Roles       []RoleViewRole
	Exits       []RoleViewExit
}

// SetRoleHolderCounter injects the counter used by RoleView for holder_count.
func (s *Service) SetRoleHolderCounter(counter RoleHolderCounter) {
	s.roleHolderCounter = counter
}

// RoleView loads the template, parses its active spec, and computes required
// roles per exit via PruneSkeletonForExit / ExitCondMet — never recomputed on
// the client (design §6 / §7).
func (s *Service) RoleView(ctx context.Context, tenantID uuid.UUID, templateKey string) (RoleView, error) {
	template, err := s.GetByKey(ctx, tenantID, templateKey)
	if err != nil {
		return RoleView{}, err
	}
	spec, err := ParseSpec(template.Spec)
	if err != nil {
		return RoleView{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return buildRoleView(ctx, s, tenantID, template, spec)
}

func buildRoleView(
	ctx context.Context,
	s *Service,
	tenantID uuid.UUID,
	template ScenarioTemplate,
	spec SpecV2,
) (RoleView, error) {
	out := RoleView{
		TemplateKey: template.Key,
		Name:        template.Name,
		Roles:       make([]RoleViewRole, 0, len(spec.Roles)),
		Exits:       make([]RoleViewExit, 0, len(spec.Exits)),
	}

	for _, role := range spec.Roles {
		caps := role.RequiredCapabilities
		if caps == nil {
			caps = []string{}
		}
		holderCount := 0
		if s != nil && s.roleHolderCounter != nil {
			n, err := s.roleHolderCounter.CountActiveHolders(ctx, tenantID, role.Key)
			if err != nil {
				return RoleView{}, err
			}
			holderCount = n
		}
		out.Roles = append(out.Roles, RoleViewRole{
			RoleKey:              role.Key,
			Title:                role.Title,
			RequiredCapabilities: caps,
			HolderCount:          holderCount,
		})
	}

	for _, exit := range spec.Exits {
		steps, err := PruneSkeletonForExit(spec, exit.Deliverable)
		if err != nil {
			out.Exits = append(out.Exits, RoleViewExit{
				Deliverable:           exit.Deliverable,
				Label:                 exit.Label,
				RequiredRoles:         []string{},
				RoleIndependencePairs: []RoleIndependencePair{},
			})
			continue
		}
		required := distinctRolesFromSteps(steps)
		pairs := independencePairsForExit(spec, exit.Deliverable, required)
		out.Exits = append(out.Exits, RoleViewExit{
			Deliverable:           exit.Deliverable,
			Label:                 exit.Label,
			RequiredRoles:         required,
			RoleIndependencePairs: pairs,
		})
	}
	return out, nil
}

func distinctRolesFromSteps(steps []SpecSkeletonStep) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, step := range steps {
		r := strings.TrimSpace(step.Role)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func independencePairsForExit(spec SpecV2, exit string, neededRoles []string) []RoleIndependencePair {
	needed := map[string]bool{}
	for _, r := range neededRoles {
		needed[r] = true
	}
	var pairs []RoleIndependencePair
	for _, c := range spec.Constraints {
		if c.Kind != "role_independence" {
			continue
		}
		if !ExitCondMet(spec, c.When, exit) {
			continue
		}
		var roles []string
		for _, rk := range c.Roles {
			if needed[rk] {
				roles = append(roles, rk)
			}
		}
		if len(roles) < 2 {
			continue
		}
		sort.Strings(roles)
		pairs = append(pairs, RoleIndependencePair{Roles: roles})
	}
	return pairs
}
