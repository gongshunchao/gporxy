package config

import (
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(r io.Reader) (Config, error) {
	var cfg Config
	err := yaml.NewDecoder(r).Decode(&cfg)
	return cfg, err
}

func LoadFile(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()

	return Load(file)
}
