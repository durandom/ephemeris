package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"howett.net/plist"
)

func LoadSkipPaths(plistPath string) ([]string, error) {
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return nil, err
	}
	return ParseSkipPaths(data)
}

func ParseSkipPaths(data []byte) ([]string, error) {
	var raw map[string]any
	if _, err := plist.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse SkipPaths: %w", err)
	}
	return skipFromValue(raw["SkipPaths"]), nil
}

func skipFromValue(v any) []string {
	var out []string
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok {
				if n := normalizePath(s); n != "" {
					out = append(out, n)
				}
			}
		}
	case []string:
		for _, s := range t {
			if n := normalizePath(s); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	return filepath.Clean(p)
}

func skipSet(paths []string) map[string]struct{} {
	set := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if n := normalizePath(p); n != "" {
			set[n] = struct{}{}
		}
	}
	return set
}
