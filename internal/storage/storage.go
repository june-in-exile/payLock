package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Backend is the seam for swapping local disk → Walrus or other storage.
type Backend interface {
	SaveUpload(id string, r io.Reader) (string, error)
	SegmentPath(id, file string) (string, error)
	ManifestPath(id string) (string, error)
	OutputDir(id string) string
}

// LocalStorage implements Backend using the local filesystem.
type LocalStorage struct {
	root string
}

func NewLocal(root string) *LocalStorage {
	return &LocalStorage{root: root}
}

// SaveUpload streams r to <root>/<id>/upload.mp4 and returns the file path.
func (l *LocalStorage) SaveUpload(id string, r io.Reader) (string, error) {
	dir := filepath.Join(l.root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	path := filepath.Join(dir, "upload.mp4")
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create upload file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("write upload file: %w", err)
	}

	return path, nil
}

// OutputDir returns the directory where FFmpeg writes HLS segments.
func (l *LocalStorage) OutputDir(id string) string {
	return filepath.Join(l.root, id)
}

// ManifestPath returns the path to the HLS manifest file.
func (l *LocalStorage) ManifestPath(id string) (string, error) {
	path := filepath.Join(l.root, id, "index.m3u8")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("manifest not found: %w", err)
	}
	return path, nil
}

// SegmentPath returns the path to a specific segment file.
func (l *LocalStorage) SegmentPath(id, file string) (string, error) {
	path := filepath.Join(l.root, id, file)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("segment not found: %w", err)
	}
	return path, nil
}

func (l *LocalStorage) List() ([]string, error) {
	entries, err := os.ReadDir(l.root)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() {
			ids = append(ids, entry.Name())
		}
	}
	return ids, nil
}

func (l *LocalStorage) HasManifest(id string) bool {
	path := filepath.Join(l.root, id, "index.m3u8")
	_, err := os.Stat(path)
	return err == nil
}

func (l *LocalStorage) HasUpload(id string) bool {
	path := filepath.Join(l.root, id, "upload.mp4")
	_, err := os.Stat(path)
	return err == nil
}
