package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}

func TestLoadValidClusterNames(t *testing.T) {
	valid := []string{"prod", "capmain-dev", "my.cluster", "cluster_01", "A123"}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			cfg := writeTestConfig(t, `
kafka:
  clusters:
    - name: `+name+`
      bootstrapServers: localhost:9092
kgazer:
  db:
    host: localhost
    port: 5432
    name: kgazer
    user: kgazer
    password: kgazer
    sslmode: disable
  server:
    port: 8080
`)
			_, err := Load(cfg)
			if err != nil {
				t.Errorf("expected valid name %q, got error: %v", name, err)
			}
		})
	}
}

func TestLoadInvalidClusterNames(t *testing.T) {
	invalid := []string{
		"has space",
		"has/slash",
		"-starts-with-dash",
		"special@char",
		"foo bar",
		"",
	}
	for _, name := range invalid {
		label := name
		if label == "" {
			label = "(empty)"
		}
		t.Run(label, func(t *testing.T) {
			content := `
kafka:
  clusters:
    - name: "` + name + `"
      bootstrapServers: localhost:9092
kgazer:
  db:
    host: localhost
    port: 5432
    name: kgazer
    user: kgazer
    password: kgazer
    sslmode: disable
  server:
    port: 8080
`
			cfg := writeTestConfig(t, content)
			_, err := Load(cfg)
			if err == nil {
				t.Errorf("expected error for invalid name %q, got nil", name)
			}
		})
	}
}
