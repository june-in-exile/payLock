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

	outputFile, err := createTempOutputPath("paylock-preview-*.mp4")
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
	f, err := os.CreateTemp("", "paylock-input-*.mp4")
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

func createTempOutputPath(prefix string) (string, error) {
	f, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", fmt.Errorf("create temp output file: %w", err)
	}
	path := f.Name()
	f.Close()
	return path, nil
}

func runFFmpegCmd(ffmpegPath, input, output string, extraArgs ...string) error {
	args := make([]string, 0, 6+len(extraArgs))
	args = append(args, "-i", input)
	args = append(args, extraArgs...)
	args = append(args, "-y", output)

	cmd := exec.Command(ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, stderr.String())
	}
	return nil
}

func runFFmpeg(ffmpegPath, input, output string, durationSec int) error {
	return runFFmpegCmd(
		ffmpegPath, input, output,
		"-t", strconv.Itoa(durationSec),
		"-c", "copy",
		"-movflags", "+faststart",
	)
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

// ExtractThumbnail extracts the first frame from MP4 data as a JPEG image.
// Returns the JPEG bytes. The input data is not mutated.
func ExtractThumbnail(data []byte, ffmpegPath string) ([]byte, error) {
	inputFile, err := writeTempInput(data)
	if err != nil {
		return nil, err
	}
	defer removeTempFile(inputFile)

	outputFile, err := createTempOutputPath("paylock-thumb-*.jpg")
	if err != nil {
		return nil, err
	}
	defer removeTempFile(outputFile)

	if err := runFFmpegCmd(ffmpegPath, inputFile, outputFile,
		"-vframes", "1",
		"-q:v", "2",
	); err != nil {
		return nil, fmt.Errorf("thumbnail extraction failed: %w", err)
	}

	thumbData, err := os.ReadFile(outputFile)
	if err != nil {
		return nil, fmt.Errorf("read thumbnail output: %w", err)
	}
	if len(thumbData) == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty thumbnail")
	}
	return thumbData, nil
}

func EnsureFaststart(data []byte, ffmpegPath string) ([]byte, error) {
	inputFile, err := writeTempInput(data)
	if err != nil {
		return nil, err
	}
	defer removeTempFile(inputFile)

	outputFile, err := createTempOutputPath("paylock-faststart-*.mp4")
	if err != nil {
		return nil, err
	}
	defer removeTempFile(outputFile)

	if err := runFFmpegCmd(ffmpegPath, inputFile, outputFile, "-c", "copy", "-movflags", "+faststart"); err != nil {
		return nil, err
	}

	return readAndValidateOutput(outputFile)
}
