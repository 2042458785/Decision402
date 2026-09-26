package probe

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type Config struct {
	DeepSeekKey    string
	DeepSeekModel  string
	DeepSeekURL    string
	APIKey         string
	NormalAddress  string
	RiskAddress    string
	AddressSource  string
	LowRiskTraits  []string
	LowRiskAddress string
}

var addressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var traitNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// LoadConfig reads a deliberately small dotenv format, without executing shell
// code or expanding variables. Process environment overrides the local file.
func LoadConfig(path string) (Config, error) {
	values := make(map[string]string)
	f, err := os.Open(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("cannot read configuration file: %w", err)
	}
	if err == nil {
		defer f.Close()
		s := bufio.NewScanner(f)
		for line := 1; s.Scan(); line++ {
			v := strings.TrimSpace(s.Text())
			if v == "" || strings.HasPrefix(v, "#") {
				continue
			}
			name, value, ok := strings.Cut(v, "=")
			if !ok {
				return Config{}, fmt.Errorf("invalid .env assignment at line %d (contents withheld)", line)
			}
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
				if len(value) < 2 || value[len(value)-1] != value[0] {
					return Config{}, fmt.Errorf("unclosed .env quote at line %d", line)
				}
				value = value[1 : len(value)-1]
			}
			if _, exists := values[name]; exists {
				return Config{}, fmt.Errorf("duplicate .env assignment at line %d", line)
			}
			values[name] = value
		}
		if err := s.Err(); err != nil {
			return Config{}, fmt.Errorf("cannot parse .env: %w", err)
		}
	}
	get := func(name string) string {
		if value, ok := os.LookupEnv(name); ok {
			return strings.TrimSpace(value)
		}
		return values[name]
	}
	cfg := Config{
		DeepSeekKey: get("DEEPSEEK_API_KEY"), DeepSeekModel: get("DEEPSEEK_MODEL"), DeepSeekURL: get("DEEPSEEK_BASE_URL"),
		APIKey: get("INTERCEPTA_API_KEY"), NormalAddress: get("INTERCEPTA_NORMAL_ADDRESS"),
		RiskAddress: get("INTERCEPTA_RISK_ADDRESS"), AddressSource: get("INTERCEPTA_ADDRESS_SOURCE"),
		LowRiskAddress: get("INTERCEPTA_LOW_RISK_ADDRESS"),
	}
	if cfg.APIKey == "" {
		return Config{}, errors.New("set INTERCEPTA_API_KEY in .env; obtain a sandbox key at https://intercepta.io/ethglobal")
	}
	if strings.ContainsAny(cfg.APIKey, " \t\r\n") {
		return Config{}, errors.New("INTERCEPTA_API_KEY contains whitespace; check the saved key")
	}
	if !addressPattern.MatchString(cfg.NormalAddress) || !addressPattern.MatchString(cfg.RiskAddress) {
		return Config{}, errors.New("set INTERCEPTA_NORMAL_ADDRESS and INTERCEPTA_RISK_ADDRESS to sponsor-provided 0x + 40 hex mainnet addresses")
	}
	if strings.EqualFold(cfg.NormalAddress, cfg.RiskAddress) {
		return Config{}, errors.New("normal and risk fixtures must be different addresses")
	}
	if cfg.LowRiskAddress != "" {
		if !addressPattern.MatchString(cfg.LowRiskAddress) || strings.EqualFold(cfg.LowRiskAddress, cfg.NormalAddress) || strings.EqualFold(cfg.LowRiskAddress, cfg.RiskAddress) {
			return Config{}, errors.New("INTERCEPTA_LOW_RISK_ADDRESS must be a distinct valid 0x address you control on the payment testnet")
		}
	}
	if cfg.AddressSource == "" {
		cfg.AddressSource = "not recorded; fixture provenance needs confirmation"
	}
	if raw := get("INTERCEPTA_LOW_RISK_TRAITS"); raw != "" {
		seen := map[string]bool{}
		for _, item := range strings.Split(raw, ",") {
			name := strings.TrimSpace(item)
			if !traitNamePattern.MatchString(name) || seen[name] {
				return Config{}, errors.New("INTERCEPTA_LOW_RISK_TRAITS must contain unique exact lowercase API trait names")
			}
			seen[name] = true
			cfg.LowRiskTraits = append(cfg.LowRiskTraits, name)
		}
	}
	return cfg, nil
}
