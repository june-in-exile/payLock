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

func TestEnsureFaststart_ValidMP4(t *testing.T) {
	data, err := os.ReadFile("../../test.mp4")
	if err != nil {
		t.Fatalf("failed to read test.mp4: %v", err)
	}

	output, err := EnsureFaststart(data, "ffmpeg")
	if err != nil {
		t.Fatalf("EnsureFaststart failed: %v", err)
	}

	if len(output) == 0 {
		t.Fatal("expected non-empty output")
	}

	if err := ValidateMagicBytes(bytes.NewReader(output)); err != nil {
		t.Errorf("output is not valid MP4: %v", err)
	}
}

func TestEnsureFaststart_HasMoovFirst(t *testing.T) {
	data, err := os.ReadFile("../../test.mp4")
	if err != nil {
		t.Fatalf("failed to read test.mp4: %v", err)
	}

	output, err := EnsureFaststart(data, "ffmpeg")
	if err != nil {
		t.Fatalf("EnsureFaststart failed: %v", err)
	}

	if !HasMoovFirst(output) {
		t.Error("expected moov atom to be first after faststart")
	}
}

func TestExtractThumbnail_ValidMP4(t *testing.T) {
	data, err := os.ReadFile("../../test.mp4")
	if err != nil {
		t.Fatalf("failed to read test.mp4: %v", err)
	}

	thumb, err := ExtractThumbnail(data, "ffmpeg")
	if err != nil {
		t.Fatalf("ExtractThumbnail failed: %v", err)
	}

	if len(thumb) == 0 {
		t.Fatal("expected non-empty thumbnail output")
	}

	// JPEG files start with FF D8
	if len(thumb) < 2 || thumb[0] != 0xFF || thumb[1] != 0xD8 {
		t.Error("expected JPEG output (FF D8 header)")
	}
}

func TestExtractThumbnail_InvalidInput(t *testing.T) {
	garbage := []byte("this is not an mp4 file at all")

	_, err := ExtractThumbnail(garbage, "ffmpeg")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestEnsureFaststart_InvalidInput(t *testing.T) {
	garbage := []byte("this is not an mp4 file at all")

	_, err := EnsureFaststart(garbage, "ffmpeg")
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}
