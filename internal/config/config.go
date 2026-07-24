package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

type DatabaseConfig struct {
	Host     string `env:"POSTGRES_HOST" env-default:"localhost"`
	Port     string `env:"POSTGRES_PORT" env-default:"5432"`
	User     string `env:"POSTGRES_USER" env-default:"sprezz_user"`
	Password string `env:"POSTGRES_PASSWORD"`
	Database string `env:"POSTGRES_DB" env-default:"sprezz"`
	SSLMode  string `env:"POSTGRES_SSLMODE" env-default:"disable"`
	URL      string `env:"DATABASE_URL"`
}

type MinIOConfig struct {
	RootUser     string `env:"MINIO_ROOT_USER" env-default:"minio_admin"`
	RootPassword string `env:"MINIO_ROOT_PASSWORD"`
	Endpoint     string `env:"MINIO_ENDPOINT" env-default:"localhost:9000"`
	UseSSL       bool   `env:"MINIO_USESSL" env-default:"false"`
	BucketName   string `env:"MINIO_BUCKET_NAME" env-default:"sprezz-media"`
}

type AppConfig struct {
	// CleanEnv requires nested structs to either have tags or have their internal tags explicitly processed
	Database      DatabaseConfig
	MinIO         MinIOConfig
	Port          string `env:"PORT" env-default:"8080"`
	// CleanEnv handles slices by splitting a comma-separated string from the environment variable
	TenantDomains []string `env:"TENANT_DOMAINS" env-separator:","`
}

func LoadConfig() (*AppConfig, error) {
	var cfg AppConfig

	// ReadEnv parses tags sequentially into the structured fields
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, fmt.Errorf("failed to read environment variables: %w", err)
	}

	// Validate configuration
	if err := validateConfig(&cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

func validateConfig(cfg *AppConfig) error {
	// Validate database configuration
	if cfg.Database.Password == "" && cfg.Database.URL == "" {
		return fmt.Errorf("either POSTGRES_PASSWORD or DATABASE_URL must be set")
	}

	// Validate MinIO configuration
	if cfg.MinIO.RootPassword == "" {
		return fmt.Errorf("MINIO_ROOT_PASSWORD must be set")
	}

	// Validate tenant configuration
	if len(cfg.TenantDomains) == 0 {
		return fmt.Errorf("at least one tenant domain must be configured (TENANT_DOMAINS)")
	}

	return nil
}

func (c *AppConfig) GetDSN() string {
	if c.Database.URL != "" {
		return c.Database.URL
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.Database.User, c.Database.Password, c.Database.Host, c.Database.Port,
		c.Database.Database, c.Database.SSLMode)
}
