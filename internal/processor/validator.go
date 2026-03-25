package processor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

// SupportedExtensions lists accepted video file extensions (lowercase, with dot).
var SupportedExtensions = map[string]bool{
	".mp4":  true,
	".mov":  true,
	".webm": true,
	".mkv":  true,
	".avi":  true,
}

var (
	ErrInvalidFormat = errors.New("invalid video format: supported formats are MP4, MOV, WebM, MKV, AVI")
	ErrFileTooLarge  = errors.New("file exceeds maximum allowed size")

	// EBML magic bytes (WebM, MKV)
	ebmlMagic = []byte{0x1A, 0x45, 0xDF, 0xA3}
)

// ValidateMagicBytes checks that the reader starts with a supported video format.
// Supported: MP4/MOV (ftyp box), WebM/MKV (EBML header), AVI (RIFF/AVI).
func ValidateMagicBytes(r io.Reader) error {
	header := make([]byte, 12)
	n, err := io.ReadFull(r, header)
	if err != nil || n < 12 {
		return ErrInvalidFormat
	}

	switch {
	case string(header[4:8]) == "ftyp":
		// MP4, MOV, and other ISO base media formats
		return nil
	case bytes.Equal(header[:4], ebmlMagic):
		// WebM, MKV (EBML container)
		return nil
	case string(header[:4]) == "RIFF" && string(header[8:12]) == "AVI ":
		// AVI
		return nil
	}

	return ErrInvalidFormat
}

var ErrInvalidThumbnailFormat = errors.New("invalid thumbnail format: expected JPEG")

// ValidateJPEGMagicBytes checks that the reader starts with JPEG magic bytes (FF D8 FF).
func ValidateJPEGMagicBytes(r io.Reader) error {
	header := make([]byte, 3)
	n, err := io.ReadFull(r, header)
	if err != nil || n < 3 {
		return ErrInvalidThumbnailFormat
	}
	if header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
		return nil
	}
	return ErrInvalidThumbnailFormat
}

func ValidateSize(size int64, maxSize int64) error {
	if size > maxSize {
		return fmt.Errorf("%w: %d bytes (max %d)", ErrFileTooLarge, size, maxSize)
	}
	return nil
}

func HasMoovFirst(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	moovIdx := bytes.Index(data, []byte("moov"))
	mdatIdx := bytes.Index(data, []byte("mdat"))
	return moovIdx > 0 && moovIdx < mdatIdx
}

type probeResult struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// ProbeFile runs ffprobe on the file and returns duration in seconds.
func ProbeFile(filepath, ffprobePath string) (float64, error) {
	cmd := exec.Command(ffprobePath,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "json",
		filepath,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var result probeResult
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, fmt.Errorf("parse ffprobe output: %w", err)
	}

	var duration float64
	if _, err := fmt.Sscanf(result.Format.Duration, "%f", &duration); err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}

	return duration, nil
}
