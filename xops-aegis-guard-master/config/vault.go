package config

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// loadVaultOverlay waits for the merged file rendered by Vault Agent and
// overlays ordinary KV plus dynamic MySQL and Redis credentials before decode.
func loadVaultOverlay(v *viper.Viper) error {
	if v == nil {
		return fmt.Errorf("config viper is nil")
	}

	explicitlyEnabled := false
	if raw := strings.TrimSpace(os.Getenv("VAULT_ENABLED")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("parse VAULT_ENABLED: %w", err)
		}
		if !enabled {
			log.Println("[Vault] disabled by VAULT_ENABLED=false; using application config")
			return nil
		}
		explicitlyEnabled = true
	}

	path := strings.TrimSpace(os.Getenv("VAULT_CREDENTIALS_FILE"))
	if path == "" {
		path = strings.TrimSpace(v.GetString("legal_tender_db.vault_credentials_file"))
	}
	if path == "" {
		path = strings.TrimSpace(v.GetString("redis.vault_credentials_file"))
	}
	if path == "" {
		if explicitlyEnabled {
			return fmt.Errorf("VAULT_ENABLED=true but Vault credentials file is empty")
		}
		return nil
	}

	v.Set("legal_tender_db.vault_credentials_file", path)
	v.Set("redis.vault_credentials_file", path)
	waitSeconds := v.GetInt("legal_tender_db.vault_initial_wait_seconds")
	if waitSeconds <= 0 {
		waitSeconds = v.GetInt("redis.vault_initial_wait_seconds")
	}
	if waitSeconds <= 0 {
		waitSeconds = 60
	}
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)

	var lastErr error
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			var values map[string]interface{}
			err = yaml.Unmarshal(data, &values)
			if err == nil {
				if len(values) == 0 {
					err = fmt.Errorf("Vault config root must be a non-empty YAML object")
				} else if err = v.MergeConfigMap(values); err == nil {
					keys := make([]string, 0, len(values))
					for key := range values {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					log.Printf("[Vault] startup KV loaded: file=%s top_level_keys=%v", path, keys)
					return nil
				}
			}
		}

		lastErr = err
		if time.Now().After(deadline) {
			return fmt.Errorf("load initial Vault config %s: %w", path, lastErr)
		}
		log.Printf("[Vault] waiting for Agent config file %s: %v", path, lastErr)
		time.Sleep(time.Second)
	}
}