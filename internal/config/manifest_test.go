package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest_Absent(t *testing.T) {
	m, err := LoadManifest(t.TempDir())
	if err != nil {
		t.Fatalf("absent manifest should not error: %v", err)
	}
	if len(m.Bootstrap.Brew) != 0 || m.Bootstrap.Shell != "" || m.Bootstrap.GitHubAuth {
		t.Errorf("absent manifest should be zero, got %+v", m)
	}
}

func TestLoadManifest_Present(t *testing.T) {
	dir := t.TempDir()
	content := `[bootstrap]
brew = ["fzf", "jq"]
backup_targets = [".zshrc"]
shell = "zsh"
github_auth = true
`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Bootstrap.Brew) != 2 || m.Bootstrap.Brew[0] != "fzf" {
		t.Errorf("brew = %v", m.Bootstrap.Brew)
	}
	if m.Bootstrap.Shell != "zsh" || !m.Bootstrap.GitHubAuth {
		t.Errorf("shell = %q, github_auth = %v", m.Bootstrap.Shell, m.Bootstrap.GitHubAuth)
	}
	if len(m.Bootstrap.BackupTargets) != 1 || m.Bootstrap.BackupTargets[0] != ".zshrc" {
		t.Errorf("backup_targets = %v", m.Bootstrap.BackupTargets)
	}
}

func TestLoadManifest_Invalid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte("not = = valid toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(dir); err == nil {
		t.Error("invalid manifest should error")
	}
}
