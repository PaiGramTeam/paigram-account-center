package config

import (
	"fmt"
	"strings"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/secretfile"
	"github.com/spf13/viper"
)

// LoadDatabase loads only the settings required by database maintenance commands.
// Runtime-only secret files and transport configuration are deliberately not read.
func LoadDatabase(paths ...string) (*Config, error) {
	v := newViper(paths)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	loaded := &Config{}
	if err := v.Unmarshal(loaded); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if path := strings.TrimSpace(loaded.Database.DSNFile); path != "" {
		value, err := secretfile.Read(path)
		if err != nil {
			return nil, fmt.Errorf("database.dsn_file: %w", err)
		}
		loaded.Database.DSN = value
	}
	if strings.TrimSpace(loaded.Database.DSN) == "" {
		return nil, fmt.Errorf("database.dsn is required")
	}
	return loaded, nil
}

// MustLoadDatabase panics when database command configuration cannot be loaded.
func MustLoadDatabase(paths ...string) *Config {
	loaded, err := LoadDatabase(paths...)
	if err != nil {
		panic(err)
	}
	return loaded
}

func newViper(paths []string) *viper.Viper {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	setDefaults(v)

	if len(paths) == 0 {
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("./configs")
	} else {
		for _, path := range paths {
			v.AddConfigPath(path)
		}
	}

	v.SetEnvPrefix("PAI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	return v
}
