package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port             string
	MaxFileSize      int64
	WalrusPublisher  string
	WalrusAggregator string
	WalrusEpochs     int
	FFmpegPath       string
	FFprobePath      string
	PreviewDuration  int
	SuiRPCURL        string
	PaywallPackageID string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:             envOrDefault("ORCA_PORT", "8080"),
		MaxFileSize:      500 * 1024 * 1024,
		WalrusPublisher:  envOrDefault("ORCA_WALRUS_PUBLISHER_URL", "https://publisher.walrus-testnet.walrus.space"),
		WalrusAggregator: envOrDefault("ORCA_WALRUS_AGGREGATOR_URL", "https://aggregator.walrus-testnet.walrus.space"),
		WalrusEpochs:     5,
		FFmpegPath:       envOrDefault("ORCA_FFMPEG_PATH", "ffmpeg"),
		FFprobePath:      envOrDefault("ORCA_FFPROBE_PATH", "ffprobe"),
		PreviewDuration:  10,
		SuiRPCURL:        envOrDefault("ORCA_SUI_RPC_URL", "https://fullnode.testnet.sui.io:443"),
		PaywallPackageID: os.Getenv("ORCA_PAYWALL_PACKAGE_ID"),
	}

	if v := os.Getenv("ORCA_MAX_FILE_SIZE_MB"); v != "" {
		mb, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ORCA_MAX_FILE_SIZE_MB: %w", err)
		}
		cfg.MaxFileSize = mb * 1024 * 1024
	}

	if v := os.Getenv("ORCA_WALRUS_EPOCHS"); v != "" {
		epochs, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ORCA_WALRUS_EPOCHS: %w", err)
		}
		cfg.WalrusEpochs = epochs
	}

	if v := os.Getenv("ORCA_PREVIEW_DURATION"); v != "" {
		dur, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid ORCA_PREVIEW_DURATION: %w", err)
		}
		cfg.PreviewDuration = dur
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
