package processor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
)

var (
	ErrInvalidFormat = errors.New("invalid video format: not an MP4 file")
	ErrFileTooLarge  = errors.New("file exceeds maximum allowed size")
)

// ValidateMagicBytes checks that the reader starts with an MP4 ftyp box.
// MP4 files have "ftyp" at byte offset 4.
func ValidateMagicBytes(r io.Reader) error {
	header := make([]byte, 12)
	n, err := io.ReadFull(r, header)
	if err != nil || n < 8 {
		return ErrInvalidFormat
	}
	// MP4: bytes 4-7 should be "ftyp"
	if string(header[4:8]) != "ftyp" {
		return ErrInvalidFormat
	}
	return nil
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
