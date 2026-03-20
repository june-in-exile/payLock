package processor

import (
	"bytes"
	"os"
	"testing"
)

func TestExtractPreview_ValidMP4(t *testing.T) {
	data, err := os.ReadFile("../../test.mp4")
	if err != nil {
		t.Fatalf("failed to read test.mp4: %v", err)
	}

	preview, err := ExtractPreview(data, 2, "ffmpeg")
	if err != nil {
		t.Fatalf("ExtractPreview failed: %v", err)
	}

	if len(preview) == 0 {
		t.Fatal("expected non-empty preview output")
	}
	if len(preview) >= len(data) {
		t.Errorf("expected preview (%d bytes) to be smaller than input (%d bytes)", len(preview), len(data))
	}

	if err := ValidateMagicBytes(bytes.NewReader(preview)); err != nil {
		t.Errorf("preview is not valid MP4: %v", err)
	}
}

func TestExtractPreview_InvalidInput(t *testing.T) {
	garbage := []byte("this is not an mp4 file at all")

	_, err := ExtractPreview(garbage, 2, "ffmpeg")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestCheckFFmpeg_Valid(t *testing.T) {
	if err := CheckFFmpeg("ffmpeg"); err != nil {
		t.Fatalf("expected ffmpeg to be found: %v", err)
	}
}

func TestCheckFFmpeg_Invalid(t *testing.T) {
	err := CheckFFmpeg("/nonexistent/ffmpeg")
	if err == nil {
		t.Fatal("expected error for nonexistent ffmpeg path")
	}
}
