package fileops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ──────────────────────────────────────
// EditFile tests
// ──────────────────────────────────────

func TestEditFile_SingleReplace(t *testing.T) {
	path := writeTemp(t, "hello world")
	svc := Service{}
	result, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: "hello", Replace: "goodbye"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "goodbye world")
	assertJSONField(t, result, "edits_count", float64(1))
}

func TestEditFile_MultipleEdits(t *testing.T) {
	path := writeTemp(t, "aaa bbb ccc")
	svc := Service{}
	result, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: "aaa", Replace: "111"},
			{Search: "ccc", Replace: "333"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "111 bbb 333")
	assertJSONField(t, result, "edits_count", float64(2))
}

func TestEditFile_SearchNotFound(t *testing.T) {
	path := writeTemp(t, "hello world")
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: "nonexistent", Replace: "x"},
		},
	})
	if err == nil {
		t.Fatal("expected error when search string not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
}

func TestEditFile_AmbiguousMatch(t *testing.T) {
	path := writeTemp(t, "foo foo bar")
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: "foo", Replace: "baz"},
		},
	})
	if err == nil {
		t.Fatal("expected error when search string is ambiguous")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected 'ambiguous' in error, got: %v", err)
	}
}

func TestEditFile_EmptySearch(t *testing.T) {
	path := writeTemp(t, "hello")
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: "", Replace: "x"},
		},
	})
	if err == nil {
		t.Fatal("expected error for empty search string")
	}
}

func TestEditFile_EmptyPath(t *testing.T) {
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path:  "",
		Edits: []EditOp{{Search: "a", Replace: "b"}},
	})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestEditFile_NoEdits(t *testing.T) {
	path := writeTemp(t, "hello")
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path:  path,
		Edits: nil,
	})
	if err == nil {
		t.Fatal("expected error for nil edits")
	}
}

func TestEditFile_FileNotExist(t *testing.T) {
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path:  filepath.Join(t.TempDir(), "no_such_file.txt"),
		Edits: []EditOp{{Search: "a", Replace: "b"}},
	})
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestEditFile_MultilineReplace(t *testing.T) {
	original := "line1\nline2\nline3\n"
	path := writeTemp(t, original)
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: "line2", Replace: "replaced_line2\nextra_line"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "line1\nreplaced_line2\nextra_line\nline3\n")
}

func TestEditFile_DeleteByReplaceEmpty(t *testing.T) {
	path := writeTemp(t, "keep_this remove_this keep_that")
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: " remove_this", Replace: ""},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "keep_this keep_that")
}

func TestEditFile_SequentialEditsChained(t *testing.T) {
	// Second edit depends on result of first edit
	path := writeTemp(t, "alpha beta gamma")
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: "alpha", Replace: "ALPHA"},
			{Search: "ALPHA beta", Replace: "combined"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "combined gamma")
}

func TestEditFile_LongSearchPreview(t *testing.T) {
	path := writeTemp(t, "short content")
	longSearch := strings.Repeat("x", 120)
	svc := Service{}
	_, err := svc.EditFile(EditRequest{
		Path: path,
		Edits: []EditOp{
			{Search: longSearch, Replace: "y"},
		},
	})
	if err == nil {
		t.Fatal("expected error for non-matching long search string")
	}
	// Error message should truncate the preview
	if !strings.Contains(err.Error(), "...") {
		t.Fatalf("expected truncated preview in error, got: %v", err)
	}
}

// ──────────────────────────────────────
// Read tests
// ──────────────────────────────────────

func TestRead_Basic(t *testing.T) {
	path := writeTemp(t, "hello world")
	svc := Service{}
	content, truncated, err := svc.Read(ReadRequest{Path: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "hello world" {
		t.Fatalf("expected 'hello world', got %q", content)
	}
	if truncated {
		t.Fatal("expected not truncated")
	}
}

func TestRead_Truncated(t *testing.T) {
	path := writeTemp(t, "abcdefghij")
	svc := Service{}
	content, truncated, err := svc.Read(ReadRequest{Path: path, MaxBytes: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "abcde" {
		t.Fatalf("expected 'abcde', got %q", content)
	}
	if !truncated {
		t.Fatal("expected truncated")
	}
}

func TestRead_EmptyPath(t *testing.T) {
	svc := Service{}
	_, _, err := svc.Read(ReadRequest{Path: ""})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestRead_FileNotExist(t *testing.T) {
	svc := Service{}
	_, _, err := svc.Read(ReadRequest{Path: filepath.Join(t.TempDir(), "nope.txt")})
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestRead_NoTruncationWhenMaxBytesZero(t *testing.T) {
	path := writeTemp(t, "full content")
	svc := Service{}
	content, truncated, err := svc.Read(ReadRequest{Path: path, MaxBytes: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "full content" {
		t.Fatalf("expected 'full content', got %q", content)
	}
	if truncated {
		t.Fatal("expected not truncated when MaxBytes is 0")
	}
}

// ──────────────────────────────────────
// Write tests
// ──────────────────────────────────────

func TestWrite_CreateNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new_file.txt")
	svc := Service{}
	result, err := svc.Write(WriteRequest{Path: path, Content: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "hello")
	assertJSONField(t, result, "bytes_written", float64(5))
}

func TestWrite_Overwrite(t *testing.T) {
	path := writeTemp(t, "old content")
	svc := Service{}
	_, err := svc.Write(WriteRequest{Path: path, Content: "new content"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "new content")
}

func TestWrite_Append(t *testing.T) {
	path := writeTemp(t, "first ")
	svc := Service{}
	_, err := svc.Write(WriteRequest{Path: path, Content: "second", Append: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "first second")
}

func TestWrite_CreateDirs(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "sub", "dir", "file.txt")
	svc := Service{}
	_, err := svc.Write(WriteRequest{Path: path, Content: "nested", CreateDirs: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "nested")
}

func TestWrite_EmptyPath(t *testing.T) {
	svc := Service{}
	_, err := svc.Write(WriteRequest{Path: "", Content: "x"})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

// ──────────────────────────────────────
// ListDir tests
// ──────────────────────────────────────

func TestListDir_Basic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)

	svc := Service{}
	result, truncated, err := svc.ListDir(ListDirRequest{Path: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if truncated {
		t.Fatal("unexpected truncation")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	entries, ok := parsed["entries"].([]any)
	if !ok {
		t.Fatal("entries not found in result")
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestListDir_HiddenFilesFiltered(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("h"), 0o644)
	os.WriteFile(filepath.Join(dir, "visible"), []byte("v"), 0o644)

	svc := Service{}
	result, _, err := svc.ListDir(ListDirRequest{Path: dir, ShowHidden: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal([]byte(result), &parsed)
	entries := parsed["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 visible entry, got %d", len(entries))
	}
}

func TestListDir_ShowHidden(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("h"), 0o644)
	os.WriteFile(filepath.Join(dir, "visible"), []byte("v"), 0o644)

	svc := Service{}
	result, _, err := svc.ListDir(ListDirRequest{Path: dir, ShowHidden: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal([]byte(result), &parsed)
	entries := parsed["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries with hidden, got %d", len(entries))
	}
}

func TestListDir_Limit(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, strings.Repeat("f", i+1)+".txt"), []byte("x"), 0o644)
	}

	svc := Service{}
	result, truncated, err := svc.ListDir(ListDirRequest{Path: dir, Limit: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncation when limit < entries")
	}

	var parsed map[string]any
	json.Unmarshal([]byte(result), &parsed)
	entries := parsed["entries"].([]any)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestListDir_Recursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("r"), 0o644)
	os.WriteFile(filepath.Join(sub, "child.txt"), []byte("c"), 0o644)

	svc := Service{}
	result, _, err := svc.ListDir(ListDirRequest{Path: dir, Recursive: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	json.Unmarshal([]byte(result), &parsed)
	entries := parsed["entries"].([]any)
	// Should have: sub/, root.txt, sub/child.txt
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries in recursive listing, got %d", len(entries))
	}
}

func TestListDir_NonExistentPath(t *testing.T) {
	svc := Service{}
	_, _, err := svc.ListDir(ListDirRequest{Path: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

// ──────────────────────────────────────
// Grep tests
// ──────────────────────────────────────

func TestGrep_BasicTextSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello world\nfoo bar\nhello again\n"), 0o644)

	svc := Service{}
	result, err := svc.Grep(GrepRequest{Pattern: "hello", Path: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 2 {
		t.Fatalf("expected 2 matches, got %d", parsed.Count)
	}
}

func TestGrep_RegexSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("func main() {}\nfunc helper() {}\nvar x = 1\n"), 0o644)

	svc := Service{}
	result, err := svc.Grep(GrepRequest{Pattern: `^func\s+\w+`, Path: dir, IsRegex: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 2 {
		t.Fatalf("expected 2 regex matches, got %d", parsed.Count)
	}
}

func TestGrep_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "mixed.txt"), []byte("Hello\nHELLO\nhello\nworld\n"), 0o644)

	svc := Service{}
	result, err := svc.Grep(GrepRequest{Pattern: "hello", Path: dir, CaseInsensitive: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 3 {
		t.Fatalf("expected 3 case-insensitive matches, got %d", parsed.Count)
	}
}

func TestGrep_SingleFile(t *testing.T) {
	path := writeTemp(t, "line one\nline two\nline three\n")
	svc := Service{}
	result, err := svc.Grep(GrepRequest{Pattern: "line", Path: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 3 {
		t.Fatalf("expected 3 matches in single file, got %d", parsed.Count)
	}
}

func TestGrep_IncludeGlobs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("match_me\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "data.txt"), []byte("match_me\n"), 0o644)

	svc := Service{}
	result, err := svc.Grep(GrepRequest{
		Pattern:      "match_me",
		Path:         dir,
		IncludeGlobs: []string{"*.go"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 1 {
		t.Fatalf("expected 1 match (only .go), got %d", parsed.Count)
	}
}

func TestGrep_ExcludeGlobs(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("match_me\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "debug.log"), []byte("match_me\n"), 0o644)

	svc := Service{}
	result, err := svc.Grep(GrepRequest{
		Pattern:      "match_me",
		Path:         dir,
		ExcludeGlobs: []string{"*.log"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 1 {
		t.Fatalf("expected 1 match (excluded .log), got %d", parsed.Count)
	}
}

func TestGrep_MaxMatches(t *testing.T) {
	dir := t.TempDir()
	lines := strings.Repeat("findme\n", 20)
	os.WriteFile(filepath.Join(dir, "many.txt"), []byte(lines), 0o644)

	svc := Service{}
	result, err := svc.Grep(GrepRequest{Pattern: "findme", Path: dir, MaxMatches: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 5 {
		t.Fatalf("expected 5 matches (capped), got %d", parsed.Count)
	}
	if !parsed.Truncated {
		t.Fatal("expected truncated=true")
	}
}

func TestGrep_EmptyPattern(t *testing.T) {
	svc := Service{}
	_, err := svc.Grep(GrepRequest{Pattern: "", Path: "."})
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestGrep_NonExistentPath(t *testing.T) {
	svc := Service{}
	_, err := svc.Grep(GrepRequest{Pattern: "x", Path: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

func TestGrep_SkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "text.txt"), []byte("findme\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "image.png"), []byte("findme\n"), 0o644)

	svc := Service{}
	result, err := svc.Grep(GrepRequest{Pattern: "findme", Path: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 1 {
		t.Fatalf("expected 1 match (binary skipped), got %d", parsed.Count)
	}
}

func TestGrep_RecursiveDirectorySearch(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("target\n"), 0o644)
	os.WriteFile(filepath.Join(sub, "child.txt"), []byte("target\nnope\ntarget\n"), 0o644)

	svc := Service{}
	result, err := svc.Grep(GrepRequest{Pattern: "target", Path: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	parsed := parseGrepResult(t, result)
	if parsed.Count != 3 {
		t.Fatalf("expected 3 matches across dirs, got %d", parsed.Count)
	}
}

type grepResultSummary struct {
	Count     int  `json:"count"`
	Truncated bool `json:"truncated"`
}

func parseGrepResult(t *testing.T, result string) grepResultSummary {
	t.Helper()
	var summary grepResultSummary
	if err := json.Unmarshal([]byte(result), &summary); err != nil {
		t.Fatalf("failed to parse grep result: %v", err)
	}
	return summary
}

// ──────────────────────────────────────
// Delete tests
// ──────────────────────────────────────

func TestDelete_File(t *testing.T) {
	path := writeTemp(t, "delete me")
	svc := Service{}
	result, err := svc.Delete(DeleteRequest{Path: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file to be deleted")
	}
}

func TestDelete_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "empty_sub")
	os.Mkdir(sub, 0o755)

	svc := Service{}
	_, err := svc.Delete(DeleteRequest{Path: sub})
	if err != nil {
		t.Fatalf("unexpected error deleting empty dir: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Fatal("expected empty dir to be deleted")
	}
}

func TestDelete_DirRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "child.txt"), []byte("x"), 0o644)

	svc := Service{}
	_, err := svc.Delete(DeleteRequest{Path: sub, Recursive: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Fatal("expected dir to be recursively deleted")
	}
}

func TestDelete_NonEmptyDirWithoutRecursive(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "child.txt"), []byte("x"), 0o644)

	svc := Service{}
	_, err := svc.Delete(DeleteRequest{Path: sub, Recursive: false})
	if err == nil {
		t.Fatal("expected error deleting non-empty dir without recursive")
	}
}

func TestDelete_EmptyPath(t *testing.T) {
	svc := Service{}
	_, err := svc.Delete(DeleteRequest{Path: ""})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestDelete_NonExistent(t *testing.T) {
	svc := Service{}
	_, err := svc.Delete(DeleteRequest{Path: filepath.Join(t.TempDir(), "nope")})
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
}

// ──────────────────────────────────────
// Move tests
// ──────────────────────────────────────

func TestMove_BasicFile(t *testing.T) {
	src := writeTemp(t, "move me")
	dst := filepath.Join(t.TempDir(), "moved.txt")

	svc := Service{}
	_, err := svc.Move(MoveRequest{Src: src, Dst: dst})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("expected source to no longer exist")
	}
	assertFileContent(t, dst, "move me")
}

func TestMove_OverwriteAllowed(t *testing.T) {
	src := writeTemp(t, "new content")
	dst := writeTemp(t, "old content")

	svc := Service{}
	_, err := svc.Move(MoveRequest{Src: src, Dst: dst, Overwrite: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, dst, "new content")
}

func TestMove_OverwriteBlocked(t *testing.T) {
	src := writeTemp(t, "new content")
	dst := writeTemp(t, "old content")

	svc := Service{}
	_, err := svc.Move(MoveRequest{Src: src, Dst: dst, Overwrite: false})
	if err == nil {
		t.Fatal("expected error when overwrite is false and dst exists")
	}
}

func TestMove_CreatesParentDir(t *testing.T) {
	src := writeTemp(t, "deep move")
	dst := filepath.Join(t.TempDir(), "a", "b", "moved.txt")

	svc := Service{}
	_, err := svc.Move(MoveRequest{Src: src, Dst: dst})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, dst, "deep move")
}

func TestMove_EmptySrc(t *testing.T) {
	svc := Service{}
	_, err := svc.Move(MoveRequest{Src: "", Dst: "x"})
	if err == nil {
		t.Fatal("expected error for empty src")
	}
}

func TestMove_EmptyDst(t *testing.T) {
	svc := Service{}
	_, err := svc.Move(MoveRequest{Src: "x", Dst: ""})
	if err == nil {
		t.Fatal("expected error for empty dst")
	}
}

// ──────────────────────────────────────
// Copy tests
// ──────────────────────────────────────

func TestCopy_File(t *testing.T) {
	src := writeTemp(t, "copy me")
	dst := filepath.Join(t.TempDir(), "copied.txt")

	svc := Service{}
	_, err := svc.Copy(CopyRequest{Src: src, Dst: dst})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, src, "copy me") // source unchanged
	assertFileContent(t, dst, "copy me") // copy created
}

func TestCopy_DirectoryRecursive(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src_dir")
	os.Mkdir(srcDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("aaa"), 0o644)
	sub := filepath.Join(srcDir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "b.txt"), []byte("bbb"), 0o644)

	dstDir := filepath.Join(dir, "dst_dir")

	svc := Service{}
	_, err := svc.Copy(CopyRequest{Src: srcDir, Dst: dstDir, Recursive: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, filepath.Join(dstDir, "a.txt"), "aaa")
	assertFileContent(t, filepath.Join(dstDir, "sub", "b.txt"), "bbb")
}

func TestCopy_OverwriteBlocked(t *testing.T) {
	src := writeTemp(t, "new")
	dst := writeTemp(t, "old")

	svc := Service{}
	_, err := svc.Copy(CopyRequest{Src: src, Dst: dst})
	if err == nil {
		t.Fatal("expected error when dst exists and overwrite not allowed")
	}
}

func TestCopy_EmptySrc(t *testing.T) {
	svc := Service{}
	_, err := svc.Copy(CopyRequest{Src: "", Dst: "x"})
	if err == nil {
		t.Fatal("expected error for empty src")
	}
}

func TestCopy_NonExistentSrc(t *testing.T) {
	svc := Service{}
	_, err := svc.Copy(CopyRequest{Src: filepath.Join(t.TempDir(), "nope"), Dst: "x"})
	if err == nil {
		t.Fatal("expected error for non-existent src")
	}
}

// ──────────────────────────────────────
// Helpers
// ──────────────────────────────────────

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "testfile.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	if string(content) != expected {
		t.Fatalf("file content mismatch:\nexpected: %q\n     got: %q", expected, string(content))
	}
}

func assertJSONField(t *testing.T, jsonStr string, key string, expected any) {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	actual, ok := parsed[key]
	if !ok {
		t.Fatalf("key %q not found in JSON result", key)
	}
	if actual != expected {
		t.Fatalf("JSON field %q: expected %v, got %v", key, expected, actual)
	}
}
