package fileops

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type ReadRequest struct {
	Path     string
	MaxBytes int
}

type ListDirRequest struct {
	Path       string
	Recursive  bool
	ShowHidden bool
	Limit      int
}

type WriteRequest struct {
	Path       string
	Content    string
	Append     bool
	CreateDirs bool
}

type Service struct{}

type entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

func (Service) Read(request ReadRequest) (string, bool, error) {
	if request.Path == "" {
		return "", false, errors.New("path is required")
	}
	content, err := os.ReadFile(request.Path)
	if err != nil {
		return "", false, err
	}
	truncated := false
	if request.MaxBytes > 0 && len(content) > request.MaxBytes {
		content = content[:request.MaxBytes]
		truncated = true
	}
	return string(content), truncated, nil
}

func (Service) ListDir(request ListDirRequest) (string, bool, error) {
	path := request.Path
	if path == "" {
		path = "."
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 200
	}
	collected := make([]entry, 0, limit)
	truncated := false

	appendEntry := func(item entry) bool {
		if len(collected) >= limit {
			truncated = true
			return false
		}
		collected = append(collected, item)
		return true
	}

	if request.Recursive {
		err := filepath.Walk(path, func(currentPath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if currentPath == path {
				return nil
			}
			name := info.Name()
			if !request.ShowHidden && len(name) > 0 && name[0] == '.' {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !appendEntry(entry{Name: name, Path: currentPath, IsDir: info.IsDir(), Size: info.Size()}) {
				return errors.New("limit reached")
			}
			return nil
		})
		if err != nil && err.Error() != "limit reached" {
			return "", false, err
		}
	} else {
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", false, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, item := range entries {
			name := item.Name()
			if !request.ShowHidden && len(name) > 0 && name[0] == '.' {
				continue
			}
			info, err := item.Info()
			if err != nil {
				return "", false, err
			}
			if !appendEntry(entry{Name: name, Path: filepath.Join(path, name), IsDir: item.IsDir(), Size: info.Size()}) {
				break
			}
		}
	}

	payload := map[string]any{
		"path":      path,
		"recursive": request.Recursive,
		"truncated": truncated,
		"entries":   collected,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", false, err
	}
	return string(encoded), truncated, nil
}

func (Service) Write(request WriteRequest) (string, error) {
	if request.Path == "" {
		return "", errors.New("path is required")
	}

	if request.CreateDirs {
		parent := filepath.Dir(request.Path)
		if parent != "" && parent != "." {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				return "", err
			}
		}
	}

	flag := os.O_CREATE | os.O_WRONLY
	if request.Append {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	file, err := os.OpenFile(request.Path, flag, 0o644)
	if err != nil {
		return "", err
	}
	defer file.Close()

	written, err := file.WriteString(request.Content)
	if err != nil {
		return "", err
	}

	payload := map[string]any{
		"path":          request.Path,
		"bytes_written": written,
		"append":        request.Append,
		"created_dirs":  request.CreateDirs,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// EditOp represents a single search-and-replace operation.
type EditOp struct {
	Search  string `json:"search"`
	Replace string `json:"replace"`
}

// EditRequest represents a request to edit a file using search-and-replace.
type EditRequest struct {
	Path  string
	Edits []EditOp
}

// EditFile applies search-and-replace edits to a file.
// Each search string must appear exactly once in the current content.
func (Service) EditFile(request EditRequest) (string, error) {
	if request.Path == "" {
		return "", errors.New("path is required")
	}
	if len(request.Edits) == 0 {
		return "", errors.New("at least one edit is required")
	}

	raw, err := os.ReadFile(request.Path)
	if err != nil {
		return "", err
	}
	content := string(raw)

	for i, op := range request.Edits {
		if op.Search == "" {
			return "", fmt.Errorf("edit[%d]: search string is empty", i)
		}

		count := strings.Count(content, op.Search)
		if count == 0 {
			// Provide a short preview of what was searched for
			preview := op.Search
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			return "", fmt.Errorf("edit[%d]: search string not found in file: %q", i, preview)
		}
		if count > 1 {
			preview := op.Search
			if len(preview) > 80 {
				preview = preview[:80] + "..."
			}
			return "", fmt.Errorf("edit[%d]: search string is ambiguous (%d occurrences), provide more context: %q", i, count, preview)
		}

		content = strings.Replace(content, op.Search, op.Replace, 1)
	}

	if err := os.WriteFile(request.Path, []byte(content), 0o644); err != nil {
		return "", err
	}

	payload := map[string]any{
		"path":        request.Path,
		"edits_count": len(request.Edits),
		"new_size":    len(content),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// GrepRequest represents a request to search for text patterns in files.
type GrepRequest struct {
	Pattern       string   // search pattern (plain text or regex)
	Path          string   // file or directory to search
	IsRegex       bool     // treat Pattern as a regular expression
	CaseInsensitive bool   // case-insensitive search
	IncludeGlobs  []string // file globs to include (e.g. "*.go")
	ExcludeGlobs  []string // file globs to exclude (e.g. "*.log")
	MaxMatches    int      // max matches to return (0 = default 100)
	ContextLines  int      // lines of context before/after each match
}

// GrepMatch represents a single search match.
type GrepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

// Grep searches for a pattern in files under the given path.
func (Service) Grep(request GrepRequest) (string, error) {
	if request.Pattern == "" {
		return "", errors.New("pattern is required")
	}
	if request.Path == "" {
		request.Path = "."
	}
	maxMatches := request.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 100
	}

	// Compile the search pattern.
	var re *regexp.Regexp
	var err error
	if request.IsRegex {
		expr := request.Pattern
		if request.CaseInsensitive {
			expr = "(?i)" + expr
		}
		re, err = regexp.Compile(expr)
		if err != nil {
			return "", fmt.Errorf("invalid regex: %w", err)
		}
	} else {
		escaped := regexp.QuoteMeta(request.Pattern)
		if request.CaseInsensitive {
			escaped = "(?i)" + escaped
		}
		re, err = regexp.Compile(escaped)
		if err != nil {
			return "", fmt.Errorf("failed to compile pattern: %w", err)
		}
	}

	var matches []GrepMatch
	truncated := false

	info, err := os.Stat(request.Path)
	if err != nil {
		return "", err
	}

	if !info.IsDir() {
		// Single file search.
		m, err := grepFile(request.Path, re, maxMatches)
		if err != nil {
			return "", err
		}
		matches = m
		truncated = len(matches) >= maxMatches
	} else {
		// Directory walk.
		err = filepath.Walk(request.Path, func(currentPath string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil // skip unreadable entries
			}
			if fi.IsDir() {
				name := fi.Name()
				if len(name) > 0 && name[0] == '.' && currentPath != request.Path {
					return filepath.SkipDir
				}
				return nil
			}

			// Apply include/exclude filters.
			if !matchGlobs(fi.Name(), request.IncludeGlobs, true) {
				return nil
			}
			if matchGlobs(fi.Name(), request.ExcludeGlobs, false) {
				return nil
			}

			// Skip likely binary files by checking extension.
			if isBinaryExtension(fi.Name()) {
				return nil
			}

			remaining := maxMatches - len(matches)
			if remaining <= 0 {
				truncated = true
				return errors.New("limit reached")
			}

			fileMatches, err := grepFile(currentPath, re, remaining)
			if err != nil {
				return nil // skip unreadable files
			}
			matches = append(matches, fileMatches...)
			return nil
		})
		if err != nil && err.Error() != "limit reached" {
			return "", err
		}
		if len(matches) >= maxMatches {
			truncated = true
		}
	}

	payload := map[string]any{
		"pattern":   request.Pattern,
		"path":      request.Path,
		"is_regex":  request.IsRegex,
		"matches":   matches,
		"count":     len(matches),
		"truncated": truncated,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// grepFile searches a single file for the regex and returns up to limit matches.
func grepFile(path string, re *regexp.Regexp, limit int) ([]GrepMatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var matches []GrepMatch
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB per line
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, GrepMatch{
				File:    path,
				Line:    lineNum,
				Content: truncateLine(line, 500),
			})
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches, scanner.Err()
}

// truncateLine caps a line at maxLen characters to prevent oversized output.
func truncateLine(line string, maxLen int) string {
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen] + "..."
}

// matchGlobs checks if filename matches any of the given glob patterns.
// If patterns is empty, returns defaultMatch.
func matchGlobs(filename string, patterns []string, defaultMatch bool) bool {
	if len(patterns) == 0 {
		return defaultMatch
	}
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
	}
	return false
}

// isBinaryExtension returns true for file extensions that are likely binary.
func isBinaryExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".exe", ".dll", ".so", ".dylib", ".bin",
		".zip", ".tar", ".gz", ".bz2", ".7z", ".rar",
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp",
		".mp3", ".mp4", ".avi", ".mov", ".wav", ".flac",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
		".wasm", ".o", ".a", ".pyc", ".class":
		return true
	}
	return false
}

// DeleteRequest represents a request to delete a file or directory.
type DeleteRequest struct {
	Path      string
	Recursive bool
}

// Delete removes a file or directory from the filesystem.
func (Service) Delete(request DeleteRequest) (string, error) {
	if request.Path == "" {
		return "", errors.New("path is required")
	}

	info, err := os.Stat(request.Path)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		if !request.Recursive {
			return "", fmt.Errorf("path is a directory; set recursive=true to delete recursively")
		}
		if err := os.RemoveAll(request.Path); err != nil {
			return "", err
		}
	} else {
		if err := os.Remove(request.Path); err != nil {
			return "", err
		}
	}

	payload := map[string]any{
		"path":    request.Path,
		"is_dir":  info.IsDir(),
		"deleted": true,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// MoveRequest represents a request to move/rename a file or directory.
type MoveRequest struct {
	Src       string
	Dst       string
	Overwrite bool
}

// Move moves or renames a file or directory.
func (Service) Move(request MoveRequest) (string, error) {
	if request.Src == "" {
		return "", errors.New("src is required")
	}
	if request.Dst == "" {
		return "", errors.New("dst is required")
	}

	if !request.Overwrite {
		if _, err := os.Stat(request.Dst); err == nil {
			return "", fmt.Errorf("destination already exists: %s", request.Dst)
		}
	}

	// Ensure parent directory exists.
	parent := filepath.Dir(request.Dst)
	if parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return "", err
		}
	}

	if err := os.Rename(request.Src, request.Dst); err != nil {
		return "", err
	}

	payload := map[string]any{
		"src": request.Src,
		"dst": request.Dst,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// CopyRequest represents a request to copy a file or directory.
type CopyRequest struct {
	Src       string
	Dst       string
	Recursive bool
}

// Copy copies a file or directory.
func (s Service) Copy(request CopyRequest) (string, error) {
	if request.Src == "" {
		return "", errors.New("src is required")
	}
	if request.Dst == "" {
		return "", errors.New("dst is required")
	}

	info, err := os.Stat(request.Src)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		if !request.Recursive {
			return "", fmt.Errorf("source is a directory; set recursive=true to copy recursively")
		}
		if err := copyDir(request.Src, request.Dst); err != nil {
			return "", err
		}
	} else {
		if err := copyFile(request.Src, request.Dst); err != nil {
			return "", err
		}
	}

	payload := map[string]any{
		"src":   request.Src,
		"dst":   request.Dst,
		"is_dir": info.IsDir(),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func copyFile(src, dst string) error {
	parent := filepath.Dir(dst)
	if parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, srcInfo.Mode())
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}
