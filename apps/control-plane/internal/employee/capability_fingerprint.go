package employee

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/superteam/control-plane/internal/skill"
)

// capabilityManifestVersionPrefix versions the fingerprint format so the
// canonicalization can evolve without ambiguity.
const capabilityManifestVersionPrefix = "cmv1:sha256:"

type capabilityManifestSkillEntry struct {
	SkillKey              string `json:"skill_key"`
	ArchiveChecksumSHA256 string `json:"archive_checksum_sha256"`
}

type capabilityManifestMCPEntry struct {
	ServerKey        string            `json:"server_key"`
	Transport        string            `json:"transport"`
	URL              string            `json:"url"`
	AuthStrategy     string            `json:"auth_strategy"`
	CredentialEnvVar string            `json:"credential_env_var"`
	RequiredEnvVars  []string          `json:"required_env_vars"`
	HeadersEnv       map[string]string `json:"headers_env"`
	SourceScope      string            `json:"source_scope"`
	PermissionScope  string            `json:"permission_scope"`
}

type capabilityManifest struct {
	Skills     []capabilityManifestSkillEntry `json:"skills"`
	MCPServers []capabilityManifestMCPEntry   `json:"mcp_servers"`
}

// computeCapabilityManifestFingerprint fingerprints the full capability
// manifest resolved for a dispatch (skills with content checksums plus the
// effective MCP projection, project-scope bindings included). It is written
// to payload metadata as capability_manifest_version and flows into the
// attestation record; the runtime never recomputes it — its skills-only
// convergence gate is derived locally from payload.skills. Display-only
// fields (server_id, name) deliberately stay out of the fingerprint.
func computeCapabilityManifestFingerprint(skills []skill.SkillRuntimeRecord, mcpServers []RuntimeMCPServerPayload) string {
	manifest := capabilityManifest{
		Skills:     make([]capabilityManifestSkillEntry, 0, len(skills)),
		MCPServers: make([]capabilityManifestMCPEntry, 0, len(mcpServers)),
	}
	for _, s := range skills {
		manifest.Skills = append(manifest.Skills, capabilityManifestSkillEntry{
			SkillKey:              s.Slug,
			ArchiveChecksumSHA256: s.ArchiveChecksum,
		})
	}
	sort.Slice(manifest.Skills, func(i, j int) bool { return manifest.Skills[i].SkillKey < manifest.Skills[j].SkillKey })
	for _, server := range mcpServers {
		entry := capabilityManifestMCPEntry{
			ServerKey:        server.ServerKey,
			Transport:        server.Transport,
			URL:              server.URL,
			AuthStrategy:     server.AuthStrategy,
			CredentialEnvVar: server.CredentialEnvVar,
			RequiredEnvVars:  append([]string(nil), server.RequiredEnvVars...),
			HeadersEnv:       server.HeadersEnv,
			SourceScope:      server.SourceScope,
			PermissionScope:  canonicalJSONString(server.PermissionScope),
		}
		sort.Strings(entry.RequiredEnvVars)
		manifest.MCPServers = append(manifest.MCPServers, entry)
	}
	sort.Slice(manifest.MCPServers, func(i, j int) bool { return manifest.MCPServers[i].ServerKey < manifest.MCPServers[j].ServerKey })

	// encoding/json marshals map keys in sorted order, so the encoded form is
	// canonical given the explicit slice ordering above.
	encoded, err := json.Marshal(manifest)
	if err != nil {
		// Marshal of these plain types cannot fail in practice; degrade to an
		// empty-manifest fingerprint rather than blocking dispatch.
		encoded = []byte("{}")
	}
	digest := sha256.Sum256(encoded)
	return capabilityManifestVersionPrefix + hex.EncodeToString(digest[:])
}

func canonicalJSONString(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}
