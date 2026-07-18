package employee

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/skill"
)

func TestCapabilityManifestFingerprintIsOrderInsensitive(t *testing.T) {
	skillA := skill.SkillRuntimeRecord{ID: uuid.New(), Slug: "alpha", ArchiveChecksum: "sum-a"}
	skillB := skill.SkillRuntimeRecord{ID: uuid.New(), Slug: "beta", ArchiveChecksum: "sum-b"}
	mcpA := RuntimeMCPServerPayload{ServerKey: "postgres", Transport: "streamable_http", URL: "https://a", RequiredEnvVars: []string{"B_TOKEN", "A_TOKEN"}}
	mcpB := RuntimeMCPServerPayload{ServerKey: "browser", Transport: "streamable_http", URL: "https://b"}

	first := computeCapabilityManifestFingerprint([]skill.SkillRuntimeRecord{skillA, skillB}, []RuntimeMCPServerPayload{mcpA, mcpB})
	second := computeCapabilityManifestFingerprint([]skill.SkillRuntimeRecord{skillB, skillA}, []RuntimeMCPServerPayload{mcpB, {ServerKey: "postgres", Transport: "streamable_http", URL: "https://a", RequiredEnvVars: []string{"A_TOKEN", "B_TOKEN"}}})

	require.Equal(t, first, second)
	require.True(t, strings.HasPrefix(first, "cmv1:sha256:"))
}

func TestCapabilityManifestFingerprintIsContentSensitive(t *testing.T) {
	base := []skill.SkillRuntimeRecord{{Slug: "alpha", ArchiveChecksum: "sum-a"}}
	baseMCP := []RuntimeMCPServerPayload{{ServerKey: "postgres", Transport: "streamable_http", URL: "https://a"}}
	baseline := computeCapabilityManifestFingerprint(base, baseMCP)

	require.NotEqual(t, baseline, computeCapabilityManifestFingerprint(
		[]skill.SkillRuntimeRecord{{Slug: "alpha", ArchiveChecksum: "sum-a2"}}, baseMCP),
		"archive checksum change must change fingerprint")
	require.NotEqual(t, baseline, computeCapabilityManifestFingerprint(base, nil),
		"removing an MCP server must change fingerprint")
	require.NotEqual(t, baseline, computeCapabilityManifestFingerprint(base,
		[]RuntimeMCPServerPayload{{ServerKey: "postgres", Transport: "streamable_http", URL: "https://a", HeadersEnv: map[string]string{"X-Auth": "TOKEN"}}}),
		"headers_env change must change fingerprint")
	require.NotEqual(t, baseline, computeCapabilityManifestFingerprint(nil, baseMCP),
		"removing a skill must change fingerprint")
}

func TestCapabilityManifestFingerprintIgnoresDisplayFields(t *testing.T) {
	withName := computeCapabilityManifestFingerprint(nil, []RuntimeMCPServerPayload{{ServerID: uuid.NewString(), ServerKey: "postgres", Name: "生产库只读", Transport: "streamable_http", URL: "https://a"}})
	withoutName := computeCapabilityManifestFingerprint(nil, []RuntimeMCPServerPayload{{ServerID: uuid.NewString(), ServerKey: "postgres", Name: "renamed", Transport: "streamable_http", URL: "https://a"}})
	require.Equal(t, withName, withoutName)
}

func TestCapabilityManifestFingerprintEmptyManifestIsStable(t *testing.T) {
	first := computeCapabilityManifestFingerprint(nil, nil)
	second := computeCapabilityManifestFingerprint([]skill.SkillRuntimeRecord{}, []RuntimeMCPServerPayload{})
	require.Equal(t, first, second)
	require.True(t, strings.HasPrefix(first, "cmv1:sha256:"))
	require.Len(t, strings.TrimPrefix(first, "cmv1:sha256:"), 64)
}
