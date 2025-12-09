package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	VMware   VMwareConfig   `mapstructure:"vmware" validate:"required"`
	Server   ServerConfig   `mapstructure:"server" validate:"required"`
	Logging  LoggingConfig  `mapstructure:"logging" validate:"required"`
	Database DatabaseConfig `mapstructure:"database" validate:"required"`
	Storage  StorageConfig  `mapstructure:"storage" validate:"required"`
}

// VMwareConfig contains vSphere connection configuration
type VMwareConfig struct {
	VCenterURL         string        `mapstructure:"vcenter_url" validate:"required,url" example:"https://vcenter.example.com/sdk"`
	Username           string        `mapstructure:"username" validate:"required" example:"service-account"`
	Password           string        `mapstructure:"password" validate:"required" example:"secret"`
	InsecureSkipVerify bool          `mapstructure:"insecure_skip_verify" example:"false"`
	ConnectionTimeout  time.Duration `mapstructure:"connection_timeout" validate:"required" example:"30s"`
	RequestTimeout     time.Duration `mapstructure:"request_timeout" validate:"required" example:"60s"`
	RetryAttempts      int           `mapstructure:"retry_attempts" validate:"min=0,max=10" example:"3"`
	RetryDelay         time.Duration `mapstructure:"retry_delay" validate:"required" example:"5s"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Port         int           `mapstructure:"port" validate:"min=1,max=65535" example:"8080"`
	Host         string        `mapstructure:"host" example:"0.0.0.0"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" validate:"required" example:"10s"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" validate:"required" example:"10s"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout" validate:"required" example:"60s"`
	EnableCORS   bool          `mapstructure:"enable_cors" example:"true"`
	TLSConfig    TLSConfig     `mapstructure:"tls"`
}

// TLSConfig contains TLS configuration
type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled" example:"false"`
	CertFile string `mapstructure:"cert_file" example:"/path/to/cert.pem"`
	KeyFile  string `mapstructure:"key_file" example:"/path/to/key.pem"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level    string `mapstructure:"level" validate:"required,oneof=debug info warn error" example:"info"`
	Format   string `mapstructure:"format" validate:"required,oneof=json text" example:"json"`
	Output   string `mapstructure:"output" validate:"required,oneof=stdout stderr file" example:"stdout"`
	FilePath string `mapstructure:"file_path" example:"/var/log/vm-deep-inspection.log"`
}

// DatabaseConfig contains database configuration
type DatabaseConfig struct {
	Type     string `mapstructure:"type" validate:"required,oneof=sqlite postgres mysql" example:"sqlite"`
	Host     string `mapstructure:"host" example:"localhost"`
	Port     int    `mapstructure:"port" validate:"min=0,max=65535" example:"5432"`
	Name     string `mapstructure:"name" validate:"required" example:"vm_inspections"`
	User     string `mapstructure:"user" example:"postgres"`
	Password string `mapstructure:"password" example:"secret"`
	SSLMode  string `mapstructure:"ssl_mode" example:"disable"`
}

// StorageConfig contains inspection data storage configuration
type StorageConfig struct {
	BasePath string `mapstructure:"base_path" validate:"required" example:"./data/inspections"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		VMware: VMwareConfig{
			VCenterURL:         "", // Will be set via config file or env vars
			Username:           "", // Will be set via config file or env vars
			Password:           "", // Will be set via config file or env vars
			ConnectionTimeout:  30 * time.Second,
			RequestTimeout:     60 * time.Second,
			RetryAttempts:      3,
			RetryDelay:         5 * time.Second,
			InsecureSkipVerify: false,
		},
		Server: ServerConfig{
			Port:         8080,
			Host:         "0.0.0.0",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 35 * time.Minute, // Increased to accommodate long-running inspections (30 min timeout + buffer)
			IdleTimeout:  120 * time.Second,
			EnableCORS:   true,
			TLSConfig: TLSConfig{
				Enabled: false,
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Database: DatabaseConfig{
			Type:    "sqlite",
			Name:    "./data/vm_inspections.db",
			SSLMode: "disable",
		},
		Storage: StorageConfig{
			BasePath: "./data/inspections",
		},
	}
}

// Load loads configuration from multiple sources with the following precedence:
// 1. Command line flags (highest)
// 2. Environment variables
// 3. Configuration file
// 4. Default values (lowest)
func Load(configFile string) (*Config, error) {
	// Start with default configuration
	config := DefaultConfig()

	// Initialize viper
	v := viper.New()

	// Set config file
	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		// Search for config file in multiple locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./config")
		v.AddConfigPath("/etc/vm-deep-inspection/")
		v.AddConfigPath("$HOME/.vm-deep-inspection/")
	}

	// Enable environment variable support
	v.AutomaticEnv()
	v.SetEnvPrefix("VMDI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Read configuration file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, continue with defaults and env vars
	}

	// Unmarshal configuration
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// ValidateConfig validates the configuration using struct tags
func ValidateConfig(config *Config) error {
	validator := validator.New()

	// Custom validation for TLS config would go here if needed
	// For now, we'll use the additional validation functions below

	if err := validator.Struct(config); err != nil {
		return err
	}

	// Additional custom validations
	if err := validateVMwareConfig(&config.VMware); err != nil {
		return fmt.Errorf("vmware config validation failed: %w", err)
	}

	if err := validateServerConfig(&config.Server); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}

	if err := validateLoggingConfig(&config.Logging); err != nil {
		return fmt.Errorf("logging config validation failed: %w", err)
	}

	if err := validateDatabaseConfig(&config.Database); err != nil {
		return fmt.Errorf("database config validation failed: %w", err)
	}

	if err := validateStorageConfig(&config.Storage); err != nil {
		return fmt.Errorf("storage config validation failed: %w", err)
	}

	return nil
}

// validateVMwareConfig performs additional validation for VMware configuration
func validateVMwareConfig(config *VMwareConfig) error {
	if config.VCenterURL == "" {
		return fmt.Errorf("vcenter_url is required")
	}

	if config.Username == "" {
		return fmt.Errorf("username is required")
	}

	if config.Password == "" {
		return fmt.Errorf("password is required")
	}

	if config.ConnectionTimeout <= 0 {
		return fmt.Errorf("connection_timeout must be positive")
	}

	if config.RequestTimeout <= 0 {
		return fmt.Errorf("request_timeout must be positive")
	}

	return nil
}

// validateServerConfig performs additional validation for server configuration
func validateServerConfig(config *ServerConfig) error {
	if config.TLSConfig.Enabled {
		if config.TLSConfig.CertFile == "" {
			return fmt.Errorf("cert_file is required when TLS is enabled")
		}
		if config.TLSConfig.KeyFile == "" {
			return fmt.Errorf("key_file is required when TLS is enabled")
		}

		// Check if files exist
		if _, err := os.Stat(config.TLSConfig.CertFile); os.IsNotExist(err) {
			return fmt.Errorf("cert_file does not exist: %s", config.TLSConfig.CertFile)
		}
		if _, err := os.Stat(config.TLSConfig.KeyFile); os.IsNotExist(err) {
			return fmt.Errorf("key_file does not exist: %s", config.TLSConfig.KeyFile)
		}
	}

	return nil
}

// validateLoggingConfig performs additional validation for logging configuration
func validateLoggingConfig(config *LoggingConfig) error {
	if config.Output == "file" && config.FilePath == "" {
		return fmt.Errorf("file_path is required when output is set to 'file'")
	}

	return nil
}

// validateDatabaseConfig performs additional validation for database configuration
func validateDatabaseConfig(config *DatabaseConfig) error {
	if config.Type == "" {
		return fmt.Errorf("database type is required")
	}

	if config.Name == "" {
		return fmt.Errorf("database name is required")
	}

	// For non-sqlite databases, additional fields are required
	if config.Type != "sqlite" {
		if config.Host == "" {
			return fmt.Errorf("database host is required for %s", config.Type)
		}
		if config.User == "" {
			return fmt.Errorf("database user is required for %s", config.Type)
		}
		if config.Port == 0 {
			return fmt.Errorf("database port is required for %s", config.Type)
		}
	}

	return nil
}

// validateStorageConfig performs additional validation for storage configuration
func validateStorageConfig(config *StorageConfig) error {
	if config.BasePath == "" {
		return fmt.Errorf("base_path is required")
	}

	return nil
}

// GetAddress returns the server address in host:port format
func (c *ServerConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// IsTLSEnabled returns true if TLS is enabled
func (c *ServerConfig) IsTLSEnabled() bool {
	return c.TLSConfig.Enabled
}

// GetDSN returns the database DSN (Data Source Name) for GORM
func (c *DatabaseConfig) GetDSN() string {
	switch c.Type {
	case "sqlite":
		return c.Name
	case "postgres":
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)
	case "mysql":
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			c.User, c.Password, c.Host, c.Port, c.Name)
	default:
		return ""
	}
}