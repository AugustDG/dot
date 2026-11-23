package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/term"
)

// DetectOS returns the current operating system identifier ("darwin" or "linux").
func DetectOS() string {
	return runtime.GOOS
}

// HomeDir returns the current user's home directory.
func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// ExpandHome replaces a leading ~ with the user's home directory.
func ExpandHome(path string) string {
	if path == "~" {
		return HomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(HomeDir(), path[2:])
	}
	return path
}

// DefaultDotfilesDir is where `dot init` puts the repo when no --dir is given.
func DefaultDotfilesDir() string {
	return filepath.Join(HomeDir(), ".dotfiles")
}

// userConfig is the CLI's own settings file, written by `dot init`.
type userConfig struct {
	Dir string `toml:"dir"`
}

// ConfigFile returns the path of the CLI's settings file:
// $XDG_CONFIG_HOME/dot/config.toml, or ~/.config/dot/config.toml.
func ConfigFile() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(HomeDir(), ".config")
	}
	return filepath.Join(base, "dot", "config.toml")
}

// DotfilesDir returns the dotfiles repo directory: $DOT_DIR if set, else the
// dir saved in ConfigFile by `dot init`, else DefaultDotfilesDir.
func DotfilesDir() string {
	if dir := os.Getenv("DOT_DIR"); dir != "" {
		return ExpandHome(dir)
	}
	var cfg userConfig
	if _, err := toml.DecodeFile(ConfigFile(), &cfg); err == nil && cfg.Dir != "" {
		return ExpandHome(cfg.Dir)
	}
	return DefaultDotfilesDir()
}

// SaveDotfilesDir records dir in ConfigFile so later commands find the repo.
func SaveDotfilesDir(dir string) error {
	path := ConfigFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(userConfig{Dir: dir}); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

// LocalBin returns ~/.local/bin, the install target for user binaries.
func LocalBin() string {
	return filepath.Join(HomeDir(), ".local", "bin")
}

// Editor resolves the user's terminal editor from $VISUAL, then $EDITOR,
// falling back to the first of nvim/vim/vi/nano found on PATH. Returns "" if
// none is available.
func Editor() string {
	for _, env := range []string{os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if env != "" {
			return env
		}
	}
	for _, candidate := range []string{"nvim", "vim", "vi", "nano"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// PathContains reports whether dir is one of the entries in $PATH.
func PathContains(dir string) bool {
	dir = filepath.Clean(dir)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(entry) == dir {
			return true
		}
	}
	return false
}

// IsInteractive returns true if both stdin and stdout are terminals, so the
// CLI can read keys and render a live TUI.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// HasTTY returns true if /dev/tty is available (a real terminal exists even
// when stdin is piped, e.g. curl | bash).
func HasTTY() bool {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return false
	}
	f.Close()
	return true
}
