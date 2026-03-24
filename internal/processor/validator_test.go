package processor

import (
	"bytes"
	"testing"
)

func TestValidateMagicBytes_ValidMP4(t *testing.T) {
	// Typical MP4 header: 4 bytes size + "ftyp" + subtype
	header := make([]byte, 12)
	header[0] = 0x00
	header[1] = 0x00
	header[2] = 0x00
	header[3] = 0x1C
	copy(header[4:8], "ftyp")
	copy(header[8:12], "isom")

	err := ValidateMagicBytes(bytes.NewReader(header))
	if err != nil {
		t.Errorf("expected valid MP4, got error: %v", err)
	}
}

func TestValidateMagicBytes_ValidMOV(t *testing.T) {
	// MOV (QuickTime) also uses ftyp box with "qt  " subtype
	header := make([]byte, 12)
	header[0] = 0x00
	header[1] = 0x00
	header[2] = 0x00
	header[3] = 0x14
	copy(header[4:8], "ftyp")
	copy(header[8:12], "qt  ")

	err := ValidateMagicBytes(bytes.NewReader(header))
	if err != nil {
		t.Errorf("expected valid MOV, got error: %v", err)
	}
}

func TestValidateMagicBytes_ValidWebM(t *testing.T) {
	// WebM starts with EBML magic bytes
	header := make([]byte, 12)
	header[0] = 0x1A
	header[1] = 0x45
	header[2] = 0xDF
	header[3] = 0xA3
	// remaining bytes are EBML header content

	err := ValidateMagicBytes(bytes.NewReader(header))
	if err != nil {
		t.Errorf("expected valid WebM, got error: %v", err)
	}
}

func TestValidateMagicBytes_ValidMKV(t *testing.T) {
	// MKV uses the same EBML header as WebM
	header := make([]byte, 12)
	header[0] = 0x1A
	header[1] = 0x45
	header[2] = 0xDF
	header[3] = 0xA3

	err := ValidateMagicBytes(bytes.NewReader(header))
	if err != nil {
		t.Errorf("expected valid MKV, got error: %v", err)
	}
}

func TestValidateMagicBytes_ValidAVI(t *testing.T) {
	// AVI: "RIFF" at offset 0, file size at 4-7, "AVI " at offset 8
	header := make([]byte, 12)
	copy(header[0:4], "RIFF")
	header[4] = 0x00 // file size (irrelevant for magic check)
	header[5] = 0x00
	header[6] = 0x00
	header[7] = 0x00
	copy(header[8:12], "AVI ")

	err := ValidateMagicBytes(bytes.NewReader(header))
	if err != nil {
		t.Errorf("expected valid AVI, got error: %v", err)
	}
}

func TestValidateMagicBytes_InvalidFile(t *testing.T) {
	data := []byte("this is not a video file at all")
	err := ValidateMagicBytes(bytes.NewReader(data))
	if err != ErrInvalidFormat {
		t.Errorf("expected ErrInvalidFormat, got: %v", err)
	}
}

func TestValidateMagicBytes_TooSmall(t *testing.T) {
	data := []byte("tiny")
	err := ValidateMagicBytes(bytes.NewReader(data))
	if err != ErrInvalidFormat {
		t.Errorf("expected ErrInvalidFormat, got: %v", err)
	}
}

func TestValidateSize_OK(t *testing.T) {
	err := ValidateSize(100, 500)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidateSize_TooLarge(t *testing.T) {
	err := ValidateSize(600, 500)
	if err == nil {
		t.Fatal("expected error for oversized file")
	}
}
