package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	StreamerbotHost string            `json:"streamerbot_host"`
	StreamerbotPort string            `json:"streamerbot_port"`
	PiURL           string            `json:"pi_url"`
	Keywords        map[string]string `json:"keywords"`
}

func LoadJSONFile[T any](filePath string, v *T) error {
	data, err := os.ReadFile(filePath)

	if err != nil {
		return fmt.Errorf("error reading %s: %w", filePath, err)
	}

	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("error parsing %s: %w", filePath, err)
	}

	return nil
}

func Load() *Config {
	cfg := &Config{
		Keywords: make(map[string]string),
	}

	if err := LoadJSONFile("config.json", cfg); err != nil {
		fmt.Printf("%v\n", err)
		return cfg
	}

	fmt.Printf("Loaded config: Streamer.bot at %s:%s\n", cfg.StreamerbotHost, cfg.StreamerbotPort)
	fmt.Printf("Loaded %d keyword mappings\n", len(cfg.Keywords))

	return cfg
}
