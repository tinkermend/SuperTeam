package skill

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/superteam/control-plane/internal/apierror"
)

const maxSkillVersionLen = 80

type ArchiveEntry struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	SizeBytes   int64  `json:"size_bytes"`
	Previewable bool   `json:"previewable"`
	ContentType string `json:"content_type"`
}

type archiveInspection struct {
	Markdown    string
	FileCount   int
	Entries     []ArchiveEntry
	filesByPath map[string]*zip.File
}

func inspectSkillArchive(reader *zip.Reader) (*archiveInspection, error) {
	rootPrefix := commonRootPrefix(reader.File)
	out := &archiveInspection{filesByPath: map[string]*zip.File{}}
	for _, file := range reader.File {
		rawPath := strings.TrimPrefix(file.Name, rootPrefix)
		if isIgnoredArchiveEntry(rawPath) || isIgnoredArchiveEntry(file.Name) {
			continue
		}
		if file.FileInfo().IsDir() {
			normalized := normalizeFilePath(strings.TrimSuffix(rawPath, "/"))
			if normalized == "" {
				if strings.TrimSpace(rawPath) == "" || rawPath == "/" {
					continue
				}
				return nil, unsafeArchivePathError(file.Name)
			}
			if _, exists := out.filesByPath[normalized]; exists {
				continue
			}
			out.Entries = append(out.Entries, ArchiveEntry{
				Path:        normalized,
				Kind:        "directory",
				SizeBytes:   0,
				Previewable: false,
				ContentType: "inode/directory",
			})
			continue
		}
		normalizedPath := normalizeFilePath(rawPath)
		if normalizedPath == "" {
			return nil, unsafeArchivePathError(file.Name)
		}
		out.FileCount++
		if _, exists := out.filesByPath[normalizedPath]; exists {
			continue
		}
		out.filesByPath[normalizedPath] = file
		out.Entries = append(out.Entries, ArchiveEntry{
			Path:        normalizedPath,
			Kind:        "file",
			SizeBytes:   int64(file.UncompressedSize64),
			Previewable: isPreviewablePath(normalizedPath),
			ContentType: archiveContentType(normalizedPath),
		})
		if normalizedPath == "SKILL.md" {
			content, err := readZipFile(file)
			if err != nil {
				return nil, fmt.Errorf("%w: cannot read SKILL.md", ErrInvalidInput)
			}
			out.Markdown = content
		}
	}
	return out, nil
}

func unsafeArchivePathError(raw string) error {
	return apierror.New("skill_archive_unsafe_path", http.StatusBadRequest, "技能包包含不安全路径："+raw)
}

func readZipFile(file *zip.File) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(rc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func isPreviewablePath(value string) bool {
	ext := strings.ToLower(path.Ext(value))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".bmp", ".pdf",
		".zip", ".gz", ".tgz", ".bz2", ".xz", ".7z",
		".woff", ".woff2", ".ttf", ".otf", ".eot",
		".exe", ".dll", ".so", ".dylib", ".bin", ".wasm",
		".mp3", ".mp4", ".wav", ".webm", ".ogg":
		return false
	default:
		return true
	}
}

func archiveContentType(value string) string {
	ext := strings.ToLower(path.Ext(value))
	switch ext {
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yml", ".yaml":
		return "text/yaml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js", ".mjs":
		return "text/javascript"
	case ".ts":
		return "text/plain"
	case ".sh":
		return "text/x-shellscript"
	case ".py":
		return "text/x-python"
	case ".go":
		return "text/x-go"
	case ".rs":
		return "text/x-rust"
	case ".xml":
		return "application/xml"
	case ".toml":
		return "text/toml"
	default:
		return "text/plain"
	}
}

func truncatePreview(content string, maxBytes int64) (string, bool) {
	if maxBytes <= 0 || int64(len(content)) <= maxBytes {
		return content, false
	}
	cut := content[:maxBytes]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut, true
}

func resolveSkillVersion(formValue, frontmatter, fallback string) (string, error) {
	value := strings.TrimSpace(formValue)
	if value == "" {
		value = strings.TrimSpace(frontmatter)
	}
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	if value == "" {
		value = "v0.1.0"
	}
	if len(value) > maxSkillVersionLen {
		return "", apierror.New("skill_version_too_long", http.StatusBadRequest, "技能版本号不能超过 80 个字符")
	}
	return value, nil
}
