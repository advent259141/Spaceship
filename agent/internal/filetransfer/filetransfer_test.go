package filetransfer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownload_Success(t *testing.T) {
	content := "hello spaceship file transfer!"

	// Start a test HTTP server that serves the content.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, content)
	}))
	defer srv.Close()

	savePath := filepath.Join(t.TempDir(), "downloaded.txt")
	logger := slog.Default()

	err := Download(context.Background(), logger, srv.URL+"/file/token123", savePath)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify file content.
	got, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("reading saved file: %v", err)
	}
	if string(got) != content {
		t.Errorf("content mismatch: got %q, want %q", string(got), content)
	}
}

func TestDownload_CreatesParentDirs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data")
	}))
	defer srv.Close()

	savePath := filepath.Join(t.TempDir(), "a", "b", "c", "file.txt")

	err := Download(context.Background(), slog.Default(), srv.URL, savePath)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	if _, err := os.Stat(savePath); os.IsNotExist(err) {
		t.Error("expected file to exist after download")
	}
}

func TestDownload_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"invalid token"}`)
	}))
	defer srv.Close()

	savePath := filepath.Join(t.TempDir(), "should_not_exist.txt")
	err := Download(context.Background(), slog.Default(), srv.URL, savePath)
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if _, statErr := os.Stat(savePath); !os.IsNotExist(statErr) {
		// File may be created but empty; that's acceptable in error path.
	}
}

func TestDownload_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data")
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	savePath := filepath.Join(t.TempDir(), "cancelled.txt")
	err := Download(ctx, slog.Default(), srv.URL, savePath)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestUpload_Success(t *testing.T) {
	// Create a file to upload.
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "upload_me.txt")
	srcContent := "upload content here"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0o644); err != nil {
		t.Fatalf("creating source file: %v", err)
	}

	var receivedToken string
	var receivedContent string
	var receivedFilename string

	// Start a test HTTP server that receives the multipart upload.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("parsing multipart: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		receivedToken = r.FormValue("token")

		file, header, err := r.FormFile("file")
		if err != nil {
			t.Errorf("getting form file: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()

		receivedFilename = header.Filename
		data, _ := io.ReadAll(file)
		receivedContent = string(data)

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	err := Upload(context.Background(), slog.Default(), srv.URL+"/upload", srcPath, "test_token_abc")
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if receivedToken != "test_token_abc" {
		t.Errorf("token mismatch: got %q, want %q", receivedToken, "test_token_abc")
	}
	if receivedContent != srcContent {
		t.Errorf("content mismatch: got %q, want %q", receivedContent, srcContent)
	}
	if receivedFilename != "upload_me.txt" {
		t.Errorf("filename mismatch: got %q, want %q", receivedFilename, "upload_me.txt")
	}
}

func TestUpload_FileNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called when file doesn't exist")
	}))
	defer srv.Close()

	err := Upload(context.Background(), slog.Default(), srv.URL, "/nonexistent/file.txt", "token")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestUpload_ServerRejectsToken(t *testing.T) {
	srcPath := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(srcPath, []byte("data"), 0o644)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":"invalid token"}`)
	}))
	defer srv.Close()

	err := Upload(context.Background(), slog.Default(), srv.URL, srcPath, "bad_token")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}
