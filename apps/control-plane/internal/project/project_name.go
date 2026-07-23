package project

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// 展示名与目录名分离(2026-07-23 修订):name=展示(可中文);directory_name=磁盘相对名(ASCII 全局唯一)。
const (
	projectDirectoryNameMaxBytes = 64
	projectDirectoryNameMinLen   = 1
	projectDisplayNameMaxRunes   = 120
)

var projectDirectoryNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$|^[a-zA-Z0-9]$`)

// ValidateDisplayProjectName 校验项目展示名称(允许中文)。
func ValidateDisplayProjectName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("%w: 项目名称不能为空", ErrInvalidProjectName)
	}
	if trimmed != name {
		return fmt.Errorf("%w: 项目名称首尾不能有空白", ErrInvalidProjectName)
	}
	if utf8.RuneCountInString(trimmed) > projectDisplayNameMaxRunes {
		return fmt.Errorf("%w: 项目名称最多 %d 个字符", ErrInvalidProjectName, projectDisplayNameMaxRunes)
	}
	return nil
}

// ValidateProjectDirectoryName 校验 Runtime 工作区相对目录名。
// 创建后目录名不可改(本阶段不提供改名路径)。
func ValidateProjectDirectoryName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("%w: 项目目录名不能为空", ErrInvalidProjectName)
	}
	if trimmed != name {
		return fmt.Errorf("%w: 项目目录名首尾不能有空白", ErrInvalidProjectName)
	}
	if utf8.RuneCountInString(trimmed) != len(trimmed) {
		return fmt.Errorf("%w: 项目目录名禁止中文及其它非 ASCII 字符", ErrInvalidProjectName)
	}
	if len(trimmed) < projectDirectoryNameMinLen || len(trimmed) > projectDirectoryNameMaxBytes {
		return fmt.Errorf("%w: 项目目录名长度须为 %d–%d 字节", ErrInvalidProjectName, projectDirectoryNameMinLen, projectDirectoryNameMaxBytes)
	}
	if trimmed == "." || trimmed == ".." {
		return fmt.Errorf("%w: 项目目录名不能是 . 或 ..", ErrInvalidProjectName)
	}
	if strings.ContainsAny(trimmed, "/\\\x00") {
		return fmt.Errorf("%w: 项目目录名不能包含路径分隔符", ErrInvalidProjectName)
	}
	if !projectDirectoryNamePattern.MatchString(trimmed) {
		return fmt.Errorf("%w: 项目目录名仅允许字母、数字、点、下划线、连字符,且不能以点/连字符开头或结尾(单字符除外)", ErrInvalidProjectName)
	}
	return nil
}

// DirectoryNameFromGitURL 从 Git 远程 URL 推导默认目录名(仓库 basename,去 .git)。
func DirectoryNameFromGitURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: Git 仓库 URL 不能为空", ErrInvalidProjectName)
	}
	candidate := trimmed
	if strings.Contains(candidate, "://") {
		parsed, err := url.Parse(candidate)
		if err != nil {
			return "", fmt.Errorf("%w: 无法解析 Git URL: %v", ErrInvalidProjectName, err)
		}
		candidate = path.Base(parsed.Path)
	} else if at := strings.Index(candidate, ":"); at >= 0 && !strings.Contains(candidate[:at], "/") {
		// scp-like: git@host:org/repo.git
		candidate = path.Base(candidate[at+1:])
	} else {
		candidate = path.Base(candidate)
	}
	candidate = strings.TrimSuffix(candidate, ".git")
	candidate = strings.TrimSpace(candidate)
	if err := ValidateProjectDirectoryName(candidate); err != nil {
		return "", fmt.Errorf("%w: 无法从 URL 推导合法目录名(%q),请改用非 Git 模式并手填目录名", err, candidate)
	}
	return candidate, nil
}

// WorkspaceDirectoryName returns the on-disk relative directory for Runtime ops.
func (p Project) WorkspaceDirectoryName() string {
	if name := strings.TrimSpace(p.DirectoryName); name != "" {
		return name
	}
	return strings.TrimSpace(p.Name)
}
