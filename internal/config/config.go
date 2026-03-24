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
	FFmpegEnabled    bool
	FFmpegPath       string
	FFprobePath      string
	PreviewDuration  int
	SuiRPCURL        string
	GatingPackageID  string
	DataDir          string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:             envOrDefault("PAYLOCK_PORT", "8080"),
		MaxFileSize:      500 * 1024 * 1024,
		WalrusPublisher:  envOrDefault("PAYLOCK_WALRUS_PUBLISHER_URL", "https://publisher.walrus-testnet.walrus.space"),
		WalrusAggregator: envOrDefault("PAYLOCK_WALRUS_AGGREGATOR_URL", "https://aggregator.walrus-testnet.walrus.space"),
		WalrusEpochs:     5,
		DataDir:          envOrDefault("PAYLOCK_DATA_DIR", "data"),
		FFmpegEnabled:    envBoolOrDefault("PAYLOCK_ENABLE_FFMPEG", true),
		FFmpegPath:       envOrDefault("PAYLOCK_FFMPEG_PATH", "ffmpeg"),
		FFprobePath:      envOrDefault("PAYLOCK_FFPROBE_PATH", "ffprobe"),
		PreviewDuration:  10,
		SuiRPCURL:        envOrDefault("PAYLOCK_SUI_RPC_URL", "https://fullnode.testnet.sui.io:443"),
		GatingPackageID:  envOrDefault("PAYLOCK_GATING_PACKAGE_ID", "0xec50faf6c1bb5720d7744476282a7b22600254de3ed849808ff9aacef8ba161a"),
	}

	if v := os.Getenv("PAYLOCK_MAX_FILE_SIZE_MB"); v != "" {
		mb, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid PAYLOCK_MAX_FILE_SIZE_MB: %w", err)
		}
		cfg.MaxFileSize = mb * 1024 * 1024
	}

	if v := os.Getenv("PAYLOCK_WALRUS_EPOCHS"); v != "" {
		epochs, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid PAYLOCK_WALRUS_EPOCHS: %w", err)
		}
		cfg.WalrusEpochs = epochs
	}

	if v := os.Getenv("PAYLOCK_PREVIEW_DURATION"); v != "" {
		dur, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid PAYLOCK_PREVIEW_DURATION: %w", err)
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

func envBoolOrDefault(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	val, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return val
}
