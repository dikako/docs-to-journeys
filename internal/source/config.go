package source

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type configFile struct {
	Sources []Kind `yaml:"sources"`
}

// LoadKinds reads extra source kinds from a YAML file. A missing file is not an
// error; it just means no extra kinds.
func LoadKinds(path string) ([]Kind, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg configFile
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, k := range cfg.Sources {
		if k.Name == "" || k.Match == "" {
			return nil, fmt.Errorf("%s: every source needs name and match", path)
		}
		if _, err := k.compile(); err != nil {
			return nil, fmt.Errorf("%s: source %s: %w", path, k.Name, err)
		}
	}
	return cfg.Sources, nil
}
