package prompttemplate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PromptTemplateVariable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type PromptTemplate struct {
	ID           uuid.UUID                `json:"id"`
	TenantID     uuid.UUID                `json:"tenant_id"`
	Title        string                   `json:"title"`
	Content      string                   `json:"content"`
	CategoryCode string                   `json:"category_code"`
	Scope        string                   `json:"scope"`
	TeamID       *uuid.UUID               `json:"team_id"`
	CreatorID    uuid.UUID                `json:"creator_id"`
	Variables    []PromptTemplateVariable `json:"variables"`
	UseCount     int32                    `json:"use_count"`
	CreatedAt    time.Time                `json:"created_at"`
	UpdatedAt    time.Time                `json:"updated_at"`
}

type CreateTemplateInput struct {
	TenantID     uuid.UUID                `json:"tenant_id"`
	Title        string                   `json:"title"`
	Content      string                   `json:"content"`
	CategoryCode string                   `json:"category_code"`
	Scope        string                   `json:"scope"`
	TeamID       *uuid.UUID               `json:"team_id"`
	CreatorID    uuid.UUID                `json:"creator_id"`
	Variables    []PromptTemplateVariable `json:"variables"`
}

type Repository interface {
	List(ctx context.Context, tenantID uuid.UUID, teamIDs []uuid.UUID, userID uuid.UUID) ([]PromptTemplate, error)
	Create(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error)
	IncrementUseCount(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error
}

var tokenRegex = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_-]+)\s*\}\}`)

// Render substitutes {{name}} tokens with values and returns an error for unknown tokens.
func Render(content string, values map[string]string) (string, error) {
	var errs []string
	result := tokenRegex.ReplaceAllStringFunc(content, func(match string) string {
		submatches := tokenRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		key := submatches[1]
		val, ok := values[key]
		if !ok {
			errs = append(errs, key)
			return match
		}
		return val
	})

	if len(errs) > 0 {
		return "", fmt.Errorf("unknown tokens: %s", strings.Join(errs, ", "))
	}
	return result, nil
}
