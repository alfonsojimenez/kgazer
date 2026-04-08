package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Kafka  KafkaConfig  `yaml:"kafka"`
	KGazer KGazerConfig `yaml:"kgazer"`
}

type KafkaConfig struct {
	Clusters []Cluster `yaml:"clusters"`
}

type Cluster struct {
	Name               string            `yaml:"name"`
	BootstrapServers   string            `yaml:"bootstrapServers"`
	Properties         map[string]string `yaml:"properties"`
	SchemaRegistry     string            `yaml:"schemaRegistry"`
	SchemaRegistryAuth *Auth             `yaml:"schemaRegistryAuth"`
}

type Auth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type KGazerConfig struct {
	Server        ServerConfig  `yaml:"server"`
	DB            DBConfig      `yaml:"db"`
	Topics        []TopicConfig `yaml:"topics"`
	CompactedOnly *bool         `yaml:"compactedOnly"`
}

func (c KGazerConfig) IsCompactedOnly() bool {
	return c.CompactedOnly == nil || *c.CompactedOnly
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type DBConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslmode"`
}

func (d DBConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode)
}

type TopicConfig struct {
	Name              string `yaml:"name"`
	KeyDeserializer   string `yaml:"keyDeserializer"`
	ValueDeserializer string `yaml:"valueDeserializer"`
}

var validClusterName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(&cfg)
	for i := range cfg.Kafka.Clusters {
		c := &cfg.Kafka.Clusters[i]
		if c.Name == "" {
			return nil, fmt.Errorf("cluster at index %d has no name", i)
		}
		if !validClusterName.MatchString(c.Name) {
			return nil, fmt.Errorf("cluster name %q is invalid: must contain only letters, digits, hyphens, dots, and underscores, and start with a letter or digit", c.Name)
		}
		c.TranslateProperties()
	}
	return &cfg, nil
}

// translateJAAS converts Java-style sasl.jaas.config (used by kafbat/kafka-ui)
// into librdkafka-compatible sasl.username / sasl.password properties.
func (c *Cluster) TranslateProperties() {
	if c.Properties == nil {
		return
	}

	jaas, ok := c.Properties["sasl.jaas.config"]
	if !ok {
		return
	}

	re := regexp.MustCompile(`(?:username|user)=["']?([^"'\s;]+)["']?`)
	if m := re.FindStringSubmatch(jaas); len(m) > 1 {
		c.Properties["sasl.username"] = m[1]
	}

	re = regexp.MustCompile(`password=["']?([^"'\s;]+)["']?`)
	if m := re.FindStringSubmatch(jaas); len(m) > 1 {
		c.Properties["sasl.password"] = m[1]
	}

	delete(c.Properties, "sasl.jaas.config")

	if _, ok := c.Properties["security.protocol"]; ok {
		c.Properties["security.protocol"] = strings.ReplaceAll(
			c.Properties["security.protocol"], "SASL_PLAINTEXT", "SASL_PLAINTEXT")
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("KGAZER_DB_HOST"); v != "" {
		cfg.KGazer.DB.Host = v
	}
	if v := os.Getenv("KGAZER_DB_PASSWORD"); v != "" {
		cfg.KGazer.DB.Password = v
	}
	if v := os.Getenv("KGAZER_KAFKA_BOOTSTRAP_SERVERS"); v != "" {
		for i := range cfg.Kafka.Clusters {
			cfg.Kafka.Clusters[i].BootstrapServers = v
		}
	}
}
