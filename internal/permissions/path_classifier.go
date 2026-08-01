package permissions

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var windowsVolume = regexp.MustCompile(`^[A-Za-z]:`)

func NormalizeRelativePath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return "", errors.New("path is empty or contains NUL")
	}
	if strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, `//`) ||
		strings.HasPrefix(value, `\\?\`) || windowsVolume.MatchString(value) {
		return "", errors.New("Windows volume, UNC, and device paths are not allowed")
	}
	value = strings.ReplaceAll(value, `\`, "/")
	if path.IsAbs(value) {
		return "", errors.New("absolute paths are not allowed")
	}
	segments := strings.Split(value, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("path contains an empty, dot, or traversal segment")
		}
		if credentialSegment(segment) {
			return "", errors.New("credential paths are not allowed")
		}
	}
	normalized := path.Clean(value)
	if normalized == "." || normalized != strings.Join(segments, "/") {
		return "", errors.New("path is not canonical")
	}
	return normalized, nil
}

func ResolveWithinRoot(root string, relative string, allowMissingFinal bool) (string, error) {
	normalized, err := NormalizeRelativePath(relative)
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("permission root is missing, not a directory, or a symlink")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	current := realRoot
	segments := strings.Split(normalized, "/")
	for index, segment := range segments {
		if err := rejectCaseAlias(current, segment); err != nil {
			return "", err
		}
		next := filepath.Join(current, segment)
		info, statErr := os.Lstat(next)
		if statErr != nil {
			if os.IsNotExist(statErr) && allowMissingFinal && index == len(segments)-1 {
				current = next
				break
			}
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("permission path must not traverse a symlink")
		}
		if index < len(segments)-1 && !info.IsDir() {
			return "", errors.New("permission path parent is not a directory")
		}
		current = next
	}
	rel, err := filepath.Rel(realRoot, current)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("permission path escaped its root")
	}
	return current, nil
}

func rejectCaseAlias(parent string, requested string) error {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), requested) && entry.Name() != requested {
			return fmt.Errorf("path case alias %q does not match %q", requested, entry.Name())
		}
	}
	return nil
}

func credentialSegment(segment string) bool {
	switch strings.ToLower(segment) {
	case ".ssh", ".aws", ".gnupg", ".kube", "credentials", "credential", "id_rsa", "id_ed25519", ".env":
		return true
	default:
		return false
	}
}
