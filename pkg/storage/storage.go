package storage

import (
	"context"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/possibities/gin-boilerplate/pkg/config"
)

type FileStorage interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (string, error)
	Download(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	SignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}

func NewFileStorage(cfg *config.Config) (FileStorage, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Storage.Driver)) {
	case "local":
		return NewLocalFileStorage(cfg)
	case "s3":
		return NewS3FileStorage(cfg)
	default:
		return nil, ErrUnsupportedDriver
	}
}

func NewObjectKey(filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	if ext == "." {
		ext = ""
	}
	return uuid.NewString() + ext
}
