package processor

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

// CheckFFmpeg verifies the ffmpeg binary exists and is executable.
// Call at server startup to fail fast.
func CheckFFmpeg(ffmpegPath string) error {
	cmd := exec.Command(ffmpegPath, "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not found at %q: %w", ffmpegPath, err)
	}
	return nil
}

// ExtractPreview extracts the first durationSec seconds from MP4 data.
// Returns the preview MP4 bytes. The input data is not mutated.
func ExtractPreview(data []byte, durationSec int, ffmpegPath string) ([]byte, error) {
	inputFile, err := writeTempInput(data)
	if err != nil {
		return nil, err
	}
	defer removeTempFile(inputFile)

	outputFile, err := createTempOutputPath()
	if err != nil {
		return nil, err
	}
	defer removeTempFile(outputFile)

	if err := runFFmpeg(ffmpegPath, inputFile, outputFile, durationSec); err != nil {
		return nil, err
	}

	return readAndValidateOutput(outputFile)
}

func writeTempInput(data []byte) (string, error) {
	f, err := os.CreateTemp("", "orca-input-*.mp4")
	if err != nil {
		return "", fmt.Errorf("create temp input file: %w", err)
	}
	path := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		removeTempFile(path)
		return "", fmt.Errorf("write temp input file: %w", err)
	}
	if err := f.Close(); err != nil {
		removeTempFile(path)
		return "", fmt.Errorf("close temp input file: %w", err)
	}
	return path, nil
}

func createTempOutputPath() (string, error) {
	f, err := os.CreateTemp("", "orca-preview-*.mp4")
	if err != nil {
		return "", fmt.Errorf("create temp output file: %w", err)
	}
	path := f.Name()
	f.Close()
	return path, nil
}

func runFFmpeg(ffmpegPath, input, output string, durationSec int) error {
	cmd := exec.Command(ffmpegPath,
		"-i", input,
		"-t", strconv.Itoa(durationSec),
		"-c", "copy",
		"-movflags", "+faststart",
		"-y",
		output,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, stderr.String())
	}
	return nil
}

func readAndValidateOutput(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read preview output: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty output")
	}

	if err := ValidateMagicBytes(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("preview output is not valid MP4: %w", err)
	}
	return data, nil
}

func removeTempFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove temp file", "path", path, "error", err)
	}
}
