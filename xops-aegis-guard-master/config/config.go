package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xops-aegis-guard/logger/zaplog"
	"xops-aegis-guard/util/aes"

	"github.com/spf13/viper"
)

type Config struct {
	Server      Server               `mapstructure:"server"`
	Database    Database             `mapstructure:"legal_tender_db"`
	Redis       Redis                `mapstructure:"redis"`
	Logger      *zaplog.LoggerConfig `mapstructure:"logger"`
	Aliyun      []Aliyun             `mapstructure:"aliyun"`
	Robot       Robot                `mapstructure:"robot"`
	Telegram    Telegram             `mapstructure:"telegram"`
	Desensitize *Desensitize         `mapstructure:"desensitize"`
}

type Server struct {
	HTTPPort string `mapstructure:"http_port"`
	Debug    bool   `mapstructure:"debug"`
}

type Database struct {
	Debug           bool   `mapstructure:"debug"`
	Connection      string `mapstructure:"connection"`
	ConnMaxLifeTime int    `mapstructure:"connmaxlifetime"`
	MaxOpen         int    `mapstructure:"maxopenconns"`
	MaxIdle         int    `mapstructure:"maxidleconns"`
	Colorful        bool   `mapstructure:"colorful"`
}

type Redis struct {
	Mode     string     `mapstructure:"mode"`
	Addrs    []string   `mapstructure:"addrs"`
	Password string     `mapstructure:"password"`
	DB       int        `mapstructure:"db"`
	Pool     *RedisPool `mapstructure:"pool"`
}

type RedisPool struct {
	PoolSize           int   `mapstructure:"poolsize"`
	MinIdleConns       int   `mapstructure:"minidleconns"`
	MaxConnAge         int64 `mapstructure:"maxconnage"`
	PoolTimeout        int64 `mapstructure:"pooltimeout"`
	IdleTimeout        int64 `mapstructure:"idletimeout"`
	DialTimeout        int64 `mapstructure:"dialtimeout"`
	IdleCheckFrequency int64 `mapstructure:"idleckeckfrequency"`
}

type Aliyun struct {
	Name              string `mapstructure:"name"`
	Type              string `mapstructure:"type"`
	AccessKey         string `mapstructure:"access_key"`
	AccessKeySecret   string `mapstructure:"access_key_secret"`
	Endpoint          string `mapstructure:"endpoint"`
	CloudSIEMEndpoint string `mapstructure:"cloud_siem_endpoint"`
	RegionID          string `mapstructure:"region_id"`
}

type Robot struct {
	Enabled        bool   `mapstructure:"enabled"`
	URL            string `mapstructure:"url"`
	SourceName     string `mapstructure:"source_name"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

type Telegram struct {
	Enabled  bool   `mapstructure:"enabled"`
	BotToken string `mapstructure:"bot_token"`
	ChatID   string `mapstructure:"chat_id"`
}

type Desensitize struct {
	Enabled   bool       `mapstructure:"enabled"`
	Rules     []Rule     `mapstructure:"rules"`
	TextRules []TextRule `mapstructure:"text_rules"`
}

// TextRule describes a command password syntax. Empty Commands means all
// commands. Pattern is an optional extension with a named "password" capture.
type TextRule struct {
	Type       string   `mapstructure:"type"`
	Commands   []string `mapstructure:"commands"`
	Parameters []string `mapstructure:"parameters"`
	Pattern    string   `mapstructure:"pattern"`
}

type Rule struct {
	Name      string   `mapstructure:"name"`
	Fields    []string `mapstructure:"fields"`
	Type      string   `mapstructure:"type"`
	KeepLeft  int      `mapstructure:"keep_left"`
	KeepRight int      `mapstructure:"keep_right"`
	Replace   string   `mapstructure:"replace"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	desensitizePath := filepath.Join(filepath.Dir(path), "desensitize.yml")
	if _, err := os.Stat(desensitizePath); err == nil {
		dv := viper.New()
		dv.SetConfigFile(desensitizePath)
		if err := dv.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read desensitize config: %w", err)
		}
		var rules Desensitize
		if err := dv.Unmarshal(&rules); err != nil {
			return nil, fmt.Errorf("parse desensitize config: %w", err)
		}
		c.Desensitize = &rules
	}
	if c.Server.HTTPPort == "" {
		c.Server.HTTPPort = "8000"
	}
	if c.Logger == nil {
		c.Logger = &zaplog.LoggerConfig{}
	}
	if c.Logger.Level == "" {
		c.Logger.Level = "info"
	}
	if c.Logger.Path == "" {
		c.Logger.Path = "./logs"
	}
	if c.Logger.MaxAgeInt <= 0 {
		c.Logger.MaxAgeInt = 7
	}
	if c.Logger.RotationTimeInt <= 0 {
		c.Logger.RotationTimeInt = 24
	}
	zaplog.InitLoggerConfig(c.Logger)
	if c.Robot.TimeoutSeconds <= 0 {
		c.Robot.TimeoutSeconds = 10
	}
	if c.Database.MaxOpen <= 0 {
		c.Database.MaxOpen = 20
	}
	if c.Database.MaxIdle <= 0 {
		c.Database.MaxIdle = 5
	}
	if c.Redis.Mode == "" {
		c.Redis.Mode = "singleton"
	}
	if c.Redis.Pool == nil {
		c.Redis.Pool = &RedisPool{}
	}
	if c.Redis.Pool.PoolSize <= 0 {
		c.Redis.Pool.PoolSize = 10
	}
	if c.Redis.Pool.MinIdleConns < 0 {
		c.Redis.Pool.MinIdleConns = 0
	}
	if c.Desensitize == nil {
		c.Desensitize = &Desensitize{Enabled: true}
	}
	if c.Telegram.BotToken == "" {
		c.Telegram.BotToken = strings.TrimSpace(os.Getenv("ANQS_TELEGRAM_BOT_TOKEN"))
	}
	if c.Telegram.ChatID == "" {
		c.Telegram.ChatID = strings.TrimSpace(os.Getenv("ANQS_TELEGRAM_CHAT_ID"))
	}
	if err := decryptAliyunCredentials(&c); err != nil {
		return nil, err
	}
	_ = os.Setenv("TZ", "Asia/Shanghai")
	return &c, nil
}

func decryptAliyunCredentials(c *Config) error {
	for i := range c.Aliyun {
		account := &c.Aliyun[i]
		var err error
		account.AccessKey, err = decryptSecret(account.AccessKey)
		if err != nil {
			return fmt.Errorf("decrypt aliyun access_key for account %s: %w", account.Name, err)
		}
		account.AccessKeySecret, err = decryptSecret(account.AccessKeySecret)
		if err != nil {
			return fmt.Errorf("decrypt aliyun access_key_secret for account %s: %w", account.Name, err)
		}
	}
	return nil
}

func decryptSecret(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	result, err := aes.InitAesCrypto().Decrypt(value)
	if err != nil {
		return "", fmt.Errorf("invalid encrypted credential: %w", err)
	}
	return result, nil
}
