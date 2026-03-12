package filetransfer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Download fetches a file from the given URL and saves it to savePath.
// Used in the "upload to node" flow: AstrBot serves the file, Go agent downloads it.
func Download(ctx context.Context, logger *slog.Logger, url, savePath string) error {
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("file transfer: downloading",
		"url", url,
		"save_path", savePath,
	)
	startedAt := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	// Ensure parent directory exists.
	if dir := filepath.Dir(savePath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	f, err := os.Create(savePath)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	logger.Info("file transfer: download complete",
		"save_path", savePath,
		"bytes", written,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
	return nil
}

// Upload reads a local file and POSTs it as multipart/form-data to the given URL.
// Used in the "download from node" flow: Go agent reads the file, AstrBot receives it.
func Upload(ctx context.Context, logger *slog.Logger, uploadURL, filePath, token string) error {
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("file transfer: uploading",
		"file_path", filePath,
		"upload_url", uploadURL,
	)
	startedAt := time.Now()

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	// Use a pipe to stream the multipart body without buffering the whole file in memory.
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Write multipart in a goroutine so we can stream.
	errCh := make(chan error, 1)
	go func() {
		defer pw.Close()

		// Add the token field.
		if err := writer.WriteField("token", token); err != nil {
			errCh <- fmt.Errorf("writing token field: %w", err)
			return
		}

		// Add the file part.
		part, err := writer.CreateFormFile("file", filepath.Base(filePath))
		if err != nil {
			errCh <- fmt.Errorf("creating form file: %w", err)
			return
		}
		if _, err := io.Copy(part, f); err != nil {
			errCh <- fmt.Errorf("copying file data: %w", err)
			return
		}
		if err := writer.Close(); err != nil {
			errCh <- fmt.Errorf("closing multipart writer: %w", err)
			return
		}
		errCh <- nil
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, pr)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Wait for the writer goroutine.
	if writeErr := <-errCh; writeErr != nil {
		return writeErr
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	logger.Info("file transfer: upload complete",
		"file_path", filePath,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
	return nil
}
