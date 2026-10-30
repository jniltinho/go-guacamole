package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// DefaultConfigPaths are searched, in order, when no config file is given.
var DefaultConfigPaths = []string{"./config.toml", "/etc/goguacamole/config.toml"}

// FlagKeys maps command-line flag names to config keys.
var FlagKeys = map[string]string{
	"addr":      "app.addr",
	"log-level": "app.log_level",
	"db-driver": "db.driver",
	"db-dsn":    "db.dsn",
	"guacd":     "guacd.addr",
}

// EnvName returns the environment variable for a config key ("app.addr" → "APP_ADDR").
func EnvName(key string) string {
	return strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// Load resolves the configuration from flags, environment, a TOML file and
// defaults. configFile may be empty, in which case APP_CONFIG and the
// DefaultConfigPaths are tried.
func Load(configFile string, flags *pflag.FlagSet) (*Config, string, error) {
	v := viper.New()
	v.SetConfigType("toml")
	defaults := Defaults()
	for k, val := range defaults {
		v.SetDefault(k, val)
		if err := v.BindEnv(k, EnvName(k)); err != nil {
			return nil, "", err
		}
	}

	if configFile == "" {
		configFile = os.Getenv("APP_CONFIG")
	}
	if configFile == "" {
		for _, p := range DefaultConfigPaths {
			if _, err := os.Stat(p); err == nil {
				configFile = p
				break
			}
		}
	}
	if configFile != "" {
		raw, err := os.ReadFile(configFile)
		if err != nil {
			return nil, "", fmt.Errorf("read config file: %w", err)
		}
		if err := checkUnknownKeys(raw, defaults); err != nil {
			return nil, "", err
		}
		if err := v.ReadConfig(bytes.NewReader(raw)); err != nil {
			return nil, "", fmt.Errorf("parse config file %s: %w", configFile, err)
		}
	}

	if flags != nil {
		for name, key := range FlagKeys {
			if f := flags.Lookup(name); f != nil {
				if err := v.BindPFlag(key, f); err != nil {
					return nil, "", err
				}
			}
		}
	}

	var cfg Config
	err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		dc.ErrorUnused = true
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			stringToSliceHook,
		)
	})
	if err != nil {
		return nil, "", fmt.Errorf("decode config: %w", err)
	}
	return &cfg, configFile, cfg.Validate()
}

// stringToSliceHook splits comma-separated env values into slices.
func stringToSliceHook(from, to reflect.Type, data any) (any, error) {
	if from.Kind() != reflect.String || to != reflect.TypeOf([]string{}) {
		return data, nil
	}
	s := data.(string)
	if strings.TrimSpace(s) == "" {
		return []string{}, nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts, nil
}

func checkUnknownKeys(raw []byte, defaults map[string]any) error {
	var tree map[string]any
	if err := toml.Unmarshal(raw, &tree); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}
	var unknown []string
	for section, val := range tree {
		sub, ok := val.(map[string]any)
		if !ok {
			unknown = append(unknown, section)
			continue
		}
		for key := range sub {
			if _, ok := defaults[section+"."+key]; !ok {
				unknown = append(unknown, section+"."+key)
			}
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("unknown config key %s", strings.Join(unknown, ", "))
	}
	return nil
}

var dsnPassword = regexp.MustCompile(`(://[^:/@]+:)[^@]+(@)|(password=)\S+`)

// Redacted renders the configuration as TOML with secrets masked.
func (c *Config) Redacted() (string, error) {
	cp := *c
	mask := func(s string) string {
		if s == "" {
			return ""
		}
		return "********"
	}
	cp.App.SecretKey = mask(cp.App.SecretKey)
	cp.App.AdminPassword = mask(cp.App.AdminPassword)
	cp.Redis.Password = mask(cp.Redis.Password)
	cp.DB.DSN = dsnPassword.ReplaceAllStringFunc(cp.DB.DSN, func(m string) string {
		if strings.HasPrefix(m, "password=") {
			return "password=********"
		}
		i := strings.LastIndex(m, "@")
		j := strings.Index(m[3:], ":") + 3
		return m[:j+1] + "********" + m[i:]
	})
	// go-sql-driver DSN: user:pass@tcp(...)
	if i := strings.Index(cp.DB.DSN, "@tcp("); i > 0 && !strings.Contains(cp.DB.DSN, "://") {
		if j := strings.Index(cp.DB.DSN[:i], ":"); j > 0 {
			cp.DB.DSN = cp.DB.DSN[:j+1] + "********" + cp.DB.DSN[i:]
		}
	}

	b, err := toml.Marshal(ToMap(&cp))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ToMap flattens the config into section → key → value using mapstructure tags.
func ToMap(c *Config) map[string]map[string]any {
	out := map[string]map[string]any{}
	rv := reflect.ValueOf(c).Elem()
	for i := 0; i < rv.NumField(); i++ {
		section := rv.Type().Field(i).Tag.Get("mapstructure")
		sv := rv.Field(i)
		m := map[string]any{}
		for j := 0; j < sv.NumField(); j++ {
			key := sv.Type().Field(j).Tag.Get("mapstructure")
			val := sv.Field(j).Interface()
			if d, ok := val.(time.Duration); ok {
				val = d.String()
			}
			m[key] = val
		}
		out[section] = m
	}
	return out
}

// ErrNoConfig is returned when a required config file is missing.
var ErrNoConfig = errors.New("config file not found")
