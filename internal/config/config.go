package config

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port             string
	StorageDir       string
	MaxFileSize      int64
	FFmpegPath       string
	FFprobePath      string
	WalrusPublisher  string
	WalrusAggregator string
}

func Load() (*Config, error) {
	// Load .env if present; ignore if missing
	_ = godotenv.Load()

	cfg := &Config{
		Port:             envOrDefault("ORCA_PORT", "8080"),
		StorageDir:       envOrDefault("ORCA_STORAGE_DIR", "./storage"),
		MaxFileSize:      500 * 1024 * 1024, // 500MB
		FFmpegPath:       envOrDefault("ORCA_FFMPEG_PATH", "ffmpeg"),
		FFprobePath:      envOrDefault("ORCA_FFPROBE_PATH", "ffprobe"),
		WalrusPublisher:  envOrDefault("ORCA_WALRUS_PUBLISHER_URL", "https://publisher.walrus-testnet.walrus.space"),
		WalrusAggregator: envOrDefault("ORCA_WALRUS_AGGREGATOR_URL", "https://aggregator.walrus-testnet.walrus.space"),
	}

	if v := os.Getenv("ORCA_MAX_FILE_SIZE_MB"); v != "" {
		mb, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ORCA_MAX_FILE_SIZE_MB: %w", err)
		}
		cfg.MaxFileSize = mb * 1024 * 1024
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if _, err := exec.LookPath(c.FFmpegPath); err != nil {
		return fmt.Errorf("ffmpeg not found at %q: %w", c.FFmpegPath, err)
	}
	if _, err := exec.LookPath(c.FFprobePath); err != nil {
		return fmt.Errorf("ffprobe not found at %q: %w", c.FFprobePath, err)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
