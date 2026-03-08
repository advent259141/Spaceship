package fileops

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
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
