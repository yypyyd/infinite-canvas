package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// NewLocal stores objects as files under root. It is the zero-config backend
// for the built-in account pool: a single 幻图 container keeps generated media
// on its data volume without running RustFS.
func NewLocal(root string) *Client {
	return &Client{local: filepath.Clean(root)}
}

// Local reports whether objects are kept on the local disk.
func (c *Client) Local() bool { return c != nil && c.local != "" }

var errInvalidKey = errors.New("storage: invalid object key")

func (c *Client) localPath(key string) (string, error) {
	key = strings.TrimSpace(key)
	clean := path.Clean("/" + key)
	if key == "" || clean == "/" || strings.Contains(key, "\\") || strings.Contains(key, "\x00") || clean != "/"+strings.TrimPrefix(key, "/") {
		return "", errInvalidKey
	}
	return filepath.Join(c.local, filepath.FromSlash(strings.TrimPrefix(clean, "/"))), nil
}

func (c *Client) localCheck() error {
	info, err := os.Stat(c.local)
	if err != nil {
		return fmt.Errorf("local object storage: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local object storage: %s is not a directory", c.local)
	}
	return nil
}

func (c *Client) localEnsure() error {
	if err := os.MkdirAll(c.local, 0o755); err != nil {
		return fmt.Errorf("local object storage: %w", err)
	}
	return nil
}

func (c *Client) localPut(key string, body []byte) error {
	target, err := c.localPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".put-*")
	if err != nil {
		return err
	}
	_, writeErr := tmp.Write(body)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(writeErr, closeErr)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// localGet mirrors the S3 response shape callers already handle: 200 with the
// body, 206 for a satisfiable Range, 404 when the object is missing.
func (c *Client) localGet(key, rangeHeader string) (*http.Response, error) {
	target, err := c.localPath(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(target)
	if errors.Is(err, fs.ErrNotExist) {
		return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Header: http.Header{}, Body: http.NoBody}, nil
	}
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		file.Close()
		if err == nil {
			err = errInvalidKey
		}
		return nil, err
	}
	contentType := mime.TypeByExtension(path.Ext(key))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if strings.TrimSpace(rangeHeader) == "" {
		header := http.Header{}
		header.Set("Content-Type", contentType)
		header.Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		header.Set("Accept-Ranges", "bytes")
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: header, Body: file, ContentLength: info.Size()}, nil
	}
	defer file.Close()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Range", rangeHeader)
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", contentType)
	http.ServeContent(recorder, request, "", info.ModTime(), file)
	return recorder.Result(), nil
}

func (c *Client) localDelete(key string) error {
	target, err := c.localPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (c *Client) localList(prefix string) ([]Object, error) {
	var out []Object
	err := filepath.WalkDir(c.local, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".put-") {
			return nil
		}
		rel, err := filepath.Rel(c.local, name)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		out = append(out, Object{Key: key, Size: info.Size(), LastModified: info.ModTime()})
		return nil
	})
	return out, err
}
