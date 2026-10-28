package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DatabaseURL          string
	APIPort              int
	PaymentMockPort      int
	PaymentMockURL       string
	AnthropicAPIKey      string
	AnthropicModel       string
	ExtractTimeoutMS     int
	ExtractMaxFileMB     int
	GeocoderTimeoutMS    int
	DBLockTimeoutMS      int
	DBStatementTimeoutMS int
	DBIdleInTxTimeoutMS  int
	PriceToleranceBps    int
	OriginToleranceKM    int
	AutoApproveClean     bool
	BuyerName            string
	BuyerCountry         string
	BuyerBaseCurrency    string
	ExecuteIntervalMS    int
	ReconcileIntervalMS  int
	LeaseSeconds         int
	SamplesDir           string
}

func Load() (Config, error) {
	var errs []error
	c := Config{
		DatabaseURL:          str("DATABASE_URL", "postgres://ap:ap@localhost:5432/ap?sslmode=disable"),
		APIPort:              num("API_PORT", 3000, &errs),
		PaymentMockPort:      num("PAYMENT_MOCK_PORT", 4000, &errs),
		PaymentMockURL:       str("PAYMENT_MOCK_URL", "http://localhost:4000"),
		AnthropicAPIKey:      str("ANTHROPIC_API_KEY", ""),
		AnthropicModel:       str("ANTHROPIC_MODEL", "claude-sonnet-5-5"),
		ExtractTimeoutMS:     num("EXTRACT_TIMEOUT_MS", 60000, &errs),
		ExtractMaxFileMB:     num("EXTRACT_MAX_FILE_MB", 10, &errs),
		GeocoderTimeoutMS:    num("GEOCODER_TIMEOUT_MS", 2000, &errs),
		DBLockTimeoutMS:      num("DB_LOCK_TIMEOUT_MS", 5000, &errs),
		DBStatementTimeoutMS: num("DB_STATEMENT_TIMEOUT_MS", 10000, &errs),
		DBIdleInTxTimeoutMS:  num("DB_IDLE_IN_TX_TIMEOUT_MS", 30000, &errs),
		PriceToleranceBps:    num("PRICE_TOLERANCE_BPS", 200, &errs),
		OriginToleranceKM:    num("ORIGIN_TOLERANCE_KM", 100, &errs),
		AutoApproveClean:     boolean("AUTO_APPROVE_CLEAN", true, &errs),
		BuyerName:            str("BUYER_NAME", "Andes Retail S.A.S."),
		BuyerCountry:         str("BUYER_COUNTRY", "CO"),
		BuyerBaseCurrency:    str("BUYER_BASE_CURRENCY", "COP"),
		ExecuteIntervalMS:    num("EXECUTE_INTERVAL_MS", 5000, &errs),
		ReconcileIntervalMS:  num("RECONCILE_INTERVAL_MS", 10000, &errs),
		LeaseSeconds:         num("LEASE_SECONDS", 30, &errs),
		SamplesDir:           str("SAMPLES_DIR", "samples"),
	}
	errs = append(errs, c.validate()...)
	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid config: %w", errors.Join(errs...))
	}
	return c, nil
}

func (c Config) validate() []error {
	var errs []error
	positive := []struct {
		name  string
		value int
	}{
		{"EXTRACT_TIMEOUT_MS", c.ExtractTimeoutMS},
		{"EXTRACT_MAX_FILE_MB", c.ExtractMaxFileMB},
		{"GEOCODER_TIMEOUT_MS", c.GeocoderTimeoutMS},
		{"DB_LOCK_TIMEOUT_MS", c.DBLockTimeoutMS},
		{"DB_STATEMENT_TIMEOUT_MS", c.DBStatementTimeoutMS},
		{"DB_IDLE_IN_TX_TIMEOUT_MS", c.DBIdleInTxTimeoutMS},
		{"ORIGIN_TOLERANCE_KM", c.OriginToleranceKM},
		{"EXECUTE_INTERVAL_MS", c.ExecuteIntervalMS},
		{"RECONCILE_INTERVAL_MS", c.ReconcileIntervalMS},
		{"LEASE_SECONDS", c.LeaseSeconds},
	}
	for _, p := range positive {
		if p.value <= 0 {
			errs = append(errs, fmt.Errorf("%s must be positive, got %d", p.name, p.value))
		}
	}
	for _, p := range []struct {
		name  string
		value int
	}{{"API_PORT", c.APIPort}, {"PAYMENT_MOCK_PORT", c.PaymentMockPort}} {
		if p.value < 1 || p.value > 65535 {
			errs = append(errs, fmt.Errorf("%s must be between 1 and 65535, got %d", p.name, p.value))
		}
	}
	if u, err := url.Parse(c.PaymentMockURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("PAYMENT_MOCK_URL must be an http(s) URL, got %q", c.PaymentMockURL))
	}
	if c.PriceToleranceBps < 0 || c.PriceToleranceBps > 10000 {
		errs = append(errs, fmt.Errorf("PRICE_TOLERANCE_BPS must be between 0 and 10000, got %d", c.PriceToleranceBps))
	}
	if c.LeaseSeconds < 20 {
		errs = append(errs, fmt.Errorf("LEASE_SECONDS must be at least 20 so a slow payment (3 attempts x 3s + backoff + lookup) is never re-claimed while still running, got %d", c.LeaseSeconds))
	}
	if c.DBLockTimeoutMS >= c.DBStatementTimeoutMS || c.DBStatementTimeoutMS >= c.DBIdleInTxTimeoutMS {
		errs = append(errs, errors.New("DB timeouts must satisfy DB_LOCK_TIMEOUT_MS < DB_STATEMENT_TIMEOUT_MS < DB_IDLE_IN_TX_TIMEOUT_MS"))
	}
	if !isCode(c.BuyerCountry, 2) {
		errs = append(errs, fmt.Errorf("BUYER_COUNTRY must be a 2-letter ISO 3166 code, got %q", c.BuyerCountry))
	}
	if !isCode(c.BuyerBaseCurrency, 3) {
		errs = append(errs, fmt.Errorf("BUYER_BASE_CURRENCY must be a 3-letter ISO 4217 code, got %q", c.BuyerBaseCurrency))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.AnthropicModel == "" {
		errs = append(errs, errors.New("ANTHROPIC_MODEL must not be empty"))
	}
	return errs
}

func (c Config) FixtureMode() bool {
	return c.AnthropicAPIKey == ""
}

func isCode(s string, length int) bool {
	if len(s) != length {
		return false
	}
	return strings.ToUpper(s) == s && strings.Trim(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") == ""
}

func str(name, def string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return def
}

func num(name string, def int, errs *[]error) int {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be an integer: %w", name, err))
		return def
	}
	return n
}

func boolean(name string, def bool, errs *[]error) bool {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be true or false: %w", name, err))
		return def
	}
	return b
}
