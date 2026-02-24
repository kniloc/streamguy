package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	StreamerbotHost string
	StreamerbotPort string
	PiURL           string
	PostgresURL     string
	Keywords        map[string]string  `json:"keywords"`
	Commands        map[string]Command `json:"commands"`
}

type Command struct {
	Response string   `json:"response"`
	Aliases  []string `json:"aliases,omitempty"`
}

func LoadJSONFile[T any](filePath string, v *T) error {
	data, err := os.ReadFile(filePath)

	if err != nil {
		return fmt.Errorf("error reading %s: %w", filePath, err)
	}

	if jsonErr := json.Unmarshal(data, v); jsonErr != nil {
		return fmt.Errorf("error parsing %s: %w", filePath, jsonErr)
	}

	return nil
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		Keywords: make(map[string]string),
		Commands: make(map[string]Command),
	}

	if err := LoadJSONFile("config.json", cfg); err != nil {
		fmt.Printf("%v\n", err)
	}

	if host := os.Getenv("STREAMERBOT_HOST"); host != "" {
		cfg.StreamerbotHost = host
	}
	if port := os.Getenv("STREAMERBOT_PORT"); port != "" {
		cfg.StreamerbotPort = port
	}
	if piURL := os.Getenv("PUBLIC_PI"); piURL != "" {
		cfg.PiURL = piURL
	}
	if pgURL := os.Getenv("POSTGRES_URL"); pgURL != "" {
		cfg.PostgresURL = pgURL
	}

	fmt.Printf("Loaded config: Streamer.bot at %s:%s\n", cfg.StreamerbotHost, cfg.StreamerbotPort)
	fmt.Printf("Loaded %d keyword mappings\n", len(cfg.Keywords))

	return cfg
}
