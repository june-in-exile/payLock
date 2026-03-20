package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalStorage implements video storage using the local filesystem.
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

func (l *LocalStorage) Delete(id string) error {
	dir := filepath.Join(l.root, id)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("video directory not found: %w", err)
	}
	return os.RemoveAll(dir)
}

func (l *LocalStorage) HasUpload(id string) bool {
	path := filepath.Join(l.root, id, "upload.mp4")
	_, err := os.Stat(path)
	return err == nil
}

// Metadata holds persistable video metadata.
type Metadata struct {
	Title string `json:"title"`
}

// SaveMetadata writes metadata.json into the video's storage directory.
func (l *LocalStorage) SaveMetadata(id string, meta Metadata) error {
	dir := filepath.Join(l.root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	path := filepath.Join(dir, "metadata.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}

// LoadMetadata reads metadata.json from the video's storage directory.
// Returns empty Metadata and nil error if the file does not exist.
func (l *LocalStorage) LoadMetadata(id string) (Metadata, error) {
	path := filepath.Join(l.root, id, "metadata.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil
		}
		return Metadata{}, fmt.Errorf("read metadata: %w", err)
	}
	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return meta, nil
}
