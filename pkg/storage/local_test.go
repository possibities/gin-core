package storage

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/possibities/gin-core/pkg/config"
)

func TestLocalFileStorageUploadDownloadDelete(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Driver:        "local",
			LocalDir:      t.TempDir(),
			PublicBaseURL: "/files",
		},
	}

	store, err := NewLocalFileStorage(cfg)
	if err != nil {
		t.Fatalf("NewLocalFileStorage() error = %v", err)
	}

	key := "avatars/user-1.txt"
	if _, err := store.Upload(context.Background(), key, strings.NewReader("hello"), 5, "text/plain"); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	reader, err := store.Download(context.Background(), key)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected stored content, got %q", string(data))
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Download(context.Background(), key); err == nil {
		t.Fatal("expected deleted file to be missing")
	}
}

func TestLocalFileStorageRejectsPathTraversal(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Driver:        "local",
			LocalDir:      t.TempDir(),
			PublicBaseURL: "/files",
		},
	}

	store, err := NewLocalFileStorage(cfg)
	if err != nil {
		t.Fatalf("NewLocalFileStorage() error = %v", err)
	}

	if _, err := store.Upload(context.Background(), "../escape.txt", strings.NewReader("bad"), 3, "text/plain"); err != ErrInvalidKey {
		t.Fatalf("expected ErrInvalidKey, got %v", err)
	}
}

func TestLocalFileStorageSignedURLIncludesExpiryAndSignature(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Driver:          "local",
			LocalDir:        t.TempDir(),
			PublicBaseURL:   "/files",
			SignedURLSecret: "storage-secret",
		},
	}

	store, err := NewLocalFileStorage(cfg)
	if err != nil {
		t.Fatalf("NewLocalFileStorage() error = %v", err)
	}
	store.now = func() time.Time {
		return time.Unix(1700000000, 0).UTC()
	}

	if _, err := store.Upload(context.Background(), "docs/report.pdf", strings.NewReader("pdf"), 3, "application/pdf"); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	signedURL, err := store.SignedURL(context.Background(), "docs/report.pdf", 5*time.Minute)
	if err != nil {
		t.Fatalf("SignedURL() error = %v", err)
	}
	if !strings.Contains(signedURL, "/files/docs%2Freport.pdf?") {
		t.Fatalf("expected escaped key in signed url, got %q", signedURL)
	}
	if !strings.Contains(signedURL, "expires=") || !strings.Contains(signedURL, "signature=") {
		t.Fatalf("expected signed url query params, got %q", signedURL)
	}
}

func TestNewObjectKeyUsesUUIDWithExtension(t *testing.T) {
	key := NewObjectKey("avatar.PNG")
	if !strings.HasSuffix(key, ".png") {
		t.Fatalf("expected preserved lowercase extension, got %q", key)
	}
	if len(strings.TrimSuffix(key, ".png")) == 0 {
		t.Fatalf("expected uuid prefix, got %q", key)
	}
}

func TestLocalFileStorageRemovesPartialFileOnSizeMismatch(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Driver:        "local",
			LocalDir:      t.TempDir(),
			PublicBaseURL: "/files",
		},
	}

	store, err := NewLocalFileStorage(cfg)
	if err != nil {
		t.Fatalf("NewLocalFileStorage() error = %v", err)
	}

	if _, err := store.Upload(context.Background(), "docs/bad.txt", strings.NewReader("hello"), 4, "text/plain"); err == nil {
		t.Fatal("expected Upload() to fail on size mismatch")
	}
	if _, err := store.Download(context.Background(), "docs/bad.txt"); err == nil {
		t.Fatal("expected partial file to be removed")
	}
}
