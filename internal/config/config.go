package config

import (
	"github.com/spf13/viper"
	"github.com/user/claw2cli/internal/paths"
)

// C2CConfig holds the global c2c configuration.
type C2CConfig struct {
	// DefaultTimeout is the default timeout in seconds for skill execution.
	DefaultTimeout int `mapstructure:"default_timeout" yaml:"default_timeout"`
}

// Load reads the c2c config from ~/.c2c/config.yaml.
// If the file doesn't exist, defaults are returned.
func Load() (*C2CConfig, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(paths.BaseDir())

	// Defaults
	v.SetDefault("default_timeout", 30)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		// Config file not found is fine — use defaults
	}

	cfg := &C2CConfig{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
