package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ManifestFile is the name of the optional top-level configuration file.
const ManifestFile = "dot.toml"

// Manifest is the optional top-level dot.toml that configures what `dot init`
// sets up beyond linking modules. Every field is opt-in.
type Manifest struct {
	Bootstrap BootstrapConfig `toml:"bootstrap"`
}

// BootstrapConfig declares what the bootstrap phase installs and configures
// once the repo is cloned.
type BootstrapConfig struct {
	// Brew lists Homebrew formulae installed for the whole setup.
	Brew []string `toml:"brew"`
	// BackupTargets are files in $HOME moved aside if they exist as real
	// files (not symlinks) before stowing.
	BackupTargets []string `toml:"backup_targets"`
	// Shell is the login shell to set, by binary name (e.g. "zsh").
	Shell string `toml:"shell"`
	// GitHubAuth runs `gh auth login` when gh isn't logged in yet.
	GitHubAuth bool `toml:"github_auth"`
}

// ManifestPath returns the expected location of the manifest within dotfilesDir.
func ManifestPath(dotfilesDir string) string {
	return filepath.Join(dotfilesDir, ManifestFile)
}

// LoadManifest reads dot.toml from dotfilesDir. A missing file is not an
// error: it returns a zero Manifest, which configures nothing.
func LoadManifest(dotfilesDir string) (Manifest, error) {
	var m Manifest
	path := ManifestPath(dotfilesDir)
	if _, err := os.Stat(path); err != nil {
		return m, nil
	}
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return m, err
	}
	return m, nil
}
