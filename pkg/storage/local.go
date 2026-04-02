package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/possibities/gin-boilerplate/pkg/config"
)

var (
	ErrInvalidKey        = errors.New("invalid storage key")
	ErrUnsupportedDriver = errors.New("unsupported storage driver")
)

type LocalFileStorage struct {
	baseDir    string
	publicBase string
	signingKey []byte
	now        func() time.Time
}

func NewLocalFileStorage(cfg *config.Config) (*LocalFileStorage, error) {
	baseDir := filepath.Clean(cfg.Storage.LocalDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}

	return &LocalFileStorage{
		baseDir:    baseDir,
		publicBase: strings.TrimRight(cfg.Storage.PublicBaseURL, "/"),
		signingKey: []byte(cfg.Storage.SignedURLSecret),
		now:        time.Now,
	}, nil
}

func (s *LocalFileStorage) Upload(ctx context.Context, key string, reader io.Reader, size int64, _ string) (string, error) {
	if size < 0 {
		return "", ErrInvalidKey
	}

	target, err := s.resolvePath(key)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("create parent dir: %w", err)
	}

	file, err := os.Create(target)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	removeOnExit := true
	defer func() {
		_ = file.Close()
		if removeOnExit {
			_ = os.Remove(target)
		}
	}()

	written, err := copyWithContext(ctx, file, reader)
	if err != nil {
		return "", err
	}
	if written != size {
		return "", fmt.Errorf("unexpected file size: got %d want %d", written, size)
	}
	removeOnExit = false
	return key, nil
}

func (s *LocalFileStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
	target, err := s.resolvePath(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	return file, nil
}

func (s *LocalFileStorage) Delete(_ context.Context, key string) error {
	target, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}

func (s *LocalFileStorage) SignedURL(_ context.Context, key string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		return "", ErrInvalidKey
	}
	if _, err := s.resolvePath(key); err != nil {
		return "", err
	}

	escapedKey := url.PathEscape(key)
	if len(s.signingKey) == 0 {
		return fmt.Sprintf("%s/%s", s.publicBase, escapedKey), nil
	}

	expiresAt := s.now().Add(expiry).Unix()
	payload := key + "|" + strconv.FormatInt(expiresAt, 10)
	mac := hmac.New(sha256.New, s.signingKey)
	_, _ = mac.Write([]byte(payload))

	values := url.Values{}
	values.Set("expires", strconv.FormatInt(expiresAt, 10))
	values.Set("signature", hex.EncodeToString(mac.Sum(nil)))
	return fmt.Sprintf("%s/%s?%s", s.publicBase, escapedKey, values.Encode()), nil
}

func (s *LocalFileStorage) resolvePath(key string) (string, error) {
	cleaned := filepath.ToSlash(strings.TrimSpace(key))
	if cleaned == "" || strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, "..") {
		return "", ErrInvalidKey
	}
	target := filepath.Join(s.baseDir, filepath.FromSlash(cleaned))
	relative, err := filepath.Rel(s.baseDir, target)
	if err != nil {
		return "", ErrInvalidKey
	}
	if strings.HasPrefix(relative, "..") {
		return "", ErrInvalidKey
	}
	return target, nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		nr, er := src.Read(buffer)
		if nr > 0 {
			nw, ew := dst.Write(buffer[:nr])
			written += int64(nw)
			if ew != nil {
				return written, ew
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if errors.Is(er, io.EOF) {
				return written, nil
			}
			return written, er
		}
	}
}
