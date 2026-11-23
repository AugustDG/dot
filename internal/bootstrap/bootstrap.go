package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/AugustDG/dot/internal/brew"
	"github.com/AugustDG/dot/internal/config"
	gitops "github.com/AugustDG/dot/internal/git"
	"github.com/AugustDG/dot/internal/platform"
	"github.com/AugustDG/dot/internal/runner"
	"github.com/AugustDG/dot/internal/stow"
	"github.com/AugustDG/dot/internal/syspkg"
	"github.com/AugustDG/dot/internal/tui"
)

type Installer struct {
	dotfilesDir string
	homeDir     string
}

func NewInstaller(dotfilesDir string) *Installer {
	return &Installer{
		dotfilesDir: dotfilesDir,
		homeDir:     platform.HomeDir(),
	}
}

// SetupTasks returns the steps that install what dot itself needs and put
// the repo in place: clone repoURL, or start an empty repo when repoURL is
// empty and nothing exists yet. Every step is fatal.
func (inst *Installer) SetupTasks(repoURL string) []tui.Task {
	var tasks []tui.Task
	if runtime.GOOS == "linux" {
		tasks = append(tasks, tui.Task{
			Title:       "Install Linux prerequisites",
			Run:         func(context.Context) error { return syspkg.InstallPrereqs() },
			Interactive: needsSudo(),
			Fatal:       true,
		})
	}

	return append(tasks,
		tui.Task{
			Title: "Install Homebrew",
			Run: func(context.Context) error {
				if !brew.IsInstalled() {
					if err := brew.Install(); err != nil {
						return err
					}
				}
				brew.EnsureOnPath()
				return nil
			},
			Interactive: !brew.IsInstalled() && needsSudo(),
			Fatal:       true,
		},
		tui.Task{
			Title: "Install git and stow",
			Run:   func(context.Context) error { return brew.InstallPackages(requiredPackages) },
			Fatal: true,
		},
		tui.Task{
			Title: "Set up " + inst.dotfilesDir,
			Run:   func(context.Context) error { return inst.setUpRepo(repoURL) },
			Fatal: true,
		},
	)
}

// requiredPackages are the tools dot itself runs.
var requiredPackages = []string{"git", "stow"}

// ConfigureTasks returns the opt-in steps declared in the repo's dot.toml.
func (inst *Installer) ConfigureTasks(cfg config.BootstrapConfig) []tui.Task {
	var tasks []tui.Task
	if len(cfg.Brew) > 0 {
		tasks = append(tasks, tui.Task{
			Title: "Install " + strings.Join(cfg.Brew, ", "),
			Run:   func(context.Context) error { return brew.InstallPackages(cfg.Brew) },
		})
	}
	if cfg.GitHubAuth {
		tasks = append(tasks, tui.Task{
			Title: "GitHub auth",
			Run: func(context.Context) error {
				if gitops.IsGHAuthenticated() || !platform.HasTTY() {
					return nil
				}
				return gitops.GHAuthLogin()
			},
			Interactive: platform.HasTTY() && !gitops.IsGHAuthenticated(),
		})
	}
	if cfg.Shell != "" {
		tasks = append(tasks, tui.Task{
			Title:       "Set login shell to " + cfg.Shell,
			Run:         func(context.Context) error { return setLoginShell(cfg.Shell) },
			Interactive: needsSudo() && !LoginShellIs(cfg.Shell),
		})
	}
	if len(cfg.BackupTargets) > 0 {
		tasks = append(tasks, tui.Task{
			Title: "Back up conflicting files",
			Run:   func(context.Context) error { return backupConflicts(inst.homeDir, cfg.BackupTargets) },
		})
	}
	return tasks
}

// setUpRepo clones repoURL into the dotfiles dir, or starts an empty repo
// there when repoURL is empty, then records the dir for later commands. An
// existing repo is kept as is.
func (inst *Installer) setUpRepo(repoURL string) error {
	_, statErr := os.Stat(filepath.Join(inst.dotfilesDir, ".git"))
	switch {
	case statErr == nil:
	case repoURL != "":
		if err := gitops.CloneRepo(repoURL, inst.dotfilesDir); err != nil {
			return tui.WithHint(err, "check that "+repoURL+" exists; a private repo needs `gh auth login` (or an SSH key) first")
		}
	default:
		if err := startRepo(inst.dotfilesDir); err != nil {
			return err
		}
	}
	return platform.SaveDotfilesDir(inst.dotfilesDir)
}

// starterManifest is the dot.toml written into a new empty repo.
const starterManifest = `# What ` + "`dot init`" + ` sets up besides linking modules. Every key is optional.
[bootstrap]
# brew = ["fzf", "ripgrep"]       # Homebrew formulae for the whole setup
# backup_targets = [".zshrc"]    # $HOME files moved aside before stowing
# shell = "zsh"                  # login shell to set
# github_auth = true             # run ` + "`gh auth login`" + ` if gh isn't logged in
`

// startRepo creates an empty git repo at dir with a commented dot.toml.
func startRepo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := gitops.Init(dir); err != nil {
		return err
	}
	return os.WriteFile(config.ManifestPath(dir), []byte(starterManifest), 0o644)
}

// ModuleTasks returns the steps that link one module, grouped under its name:
// init submodules, install deps, stow, then run the post-install hook. A hook
// failure is reported as a warning since the module is already linked.
func (inst *Installer) ModuleTasks(mod config.Module, adopt bool) []tui.Task {
	group := mod.Name
	currentOS := platform.DetectOS()
	if !mod.SupportsOS(currentOS) {
		return []tui.Task{{
			Group: group,
			Title: "Link",
			Run:   func(context.Context) error { return tui.Skipped("not supported on " + currentOS) },
		}}
	}

	var tasks []tui.Task
	if mod.HasSubmodule {
		tasks = append(tasks, tui.Task{
			Group: group,
			Title: "Init submodules",
			Run: func(context.Context) error {
				return gitops.InitSubmodules(inst.dotfilesDir, mod.SubmodulePaths)
			},
		})
	}

	if !mod.Deps.Empty() {
		tasks = append(tasks, DepsTask(mod))
	}

	tasks = append(tasks, tui.Task{
		Group: group,
		Title: "Stow",
		Run: func(context.Context) error {
			if adopt {
				return stow.StowAdopt(inst.dotfilesDir, mod.Name, inst.homeDir)
			}
			err := stow.Stow(inst.dotfilesDir, mod.Name, inst.homeDir)
			var conflict *stow.ConflictError
			if errors.As(err, &conflict) {
				return tui.WithHint(err, "rerun with --adopt to absorb the existing files into the repo, or back them up first")
			}
			return err
		},
	})

	if mod.Hooks.PostInstall != "" {
		tasks = append(tasks, tui.Task{
			Group: group,
			Title: "Post-install hook",
			Run: func(ctx context.Context) error {
				cmd := exec.CommandContext(ctx, "bash", "-c", platform.ExpandHome(mod.Hooks.PostInstall))
				runner.ConfigureCmd(cmd)
				return tui.Warning(cmd.Run())
			},
		})
	}
	return tasks
}

// DepsTask returns the task that installs a module's dependencies.
func DepsTask(mod config.Module) tui.Task {
	title := "Install dependencies"
	if names := DepNames(mod.Deps); len(names) > 0 {
		title = "Install " + strings.Join(names, ", ")
	}
	return tui.Task{
		Group:       mod.Name,
		Title:       title,
		Run:         func(context.Context) error { return InstallDeps(mod.Deps) },
		Interactive: DepsNeedSudo(mod.Deps),
	}
}

func needsSudo() bool { return len(runner.SudoPrefix()) > 0 }

func backupConflicts(homeDir string, targets []string) error {
	needBackup := false
	for _, f := range targets {
		p := filepath.Join(homeDir, f)
		info, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			needBackup = true
			break
		}
	}
	if !needBackup {
		return nil
	}

	backupDir := filepath.Join(homeDir, fmt.Sprintf(".dot-backup-%s", time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	for _, f := range targets {
		src := filepath.Join(homeDir, f)
		info, err := os.Lstat(src)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		dst := filepath.Join(backupDir, f)
		// Create parent dirs so nested backup_targets (e.g. ".config/x") work.
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("backup %s: %w", f, err)
		}
		if err := os.Rename(src, dst); err != nil {
			// Cross-device move or other rename failure: copy then remove.
			if cpErr := exec.Command("cp", "-a", src, dst).Run(); cpErr != nil {
				return fmt.Errorf("backup %s: %w", f, err)
			}
			if rmErr := os.RemoveAll(src); rmErr != nil {
				return fmt.Errorf("backup %s: remove original: %w", f, rmErr)
			}
		}
	}
	return nil
}

// setLoginShell makes the named shell the login shell of the target user,
// registering it in /etc/shells first if needed.
func setLoginShell(name string) error {
	shellBin := findShell(name)
	if shellBin == "" {
		return fmt.Errorf("%s not found; add it to brew in dot.toml", name)
	}

	// Always operate on the human user (under sudo, $USER is "root"; SUDO_USER
	// holds the original account). Compare against the configured login shell,
	// not $SHELL, which reflects the currently-running shell.
	user := targetUser()
	if currentLoginShell(user) == shellBin {
		return nil
	}

	shells, _ := os.ReadFile("/etc/shells")
	if !strings.Contains(string(shells), shellBin) {
		cmd := runner.Sudo("tee", "-a", "/etc/shells")
		cmd.Stdin = strings.NewReader(shellBin + "\n")
		cmd.Stdout = nil
		cmd.Run()
	}

	// runner.Sudo prepends sudo only when not already root; passing the target
	// user explicitly makes both the root and non-root paths change the right
	// account. With no resolvable user (e.g. root in a minimal container),
	// fall back to a no-username chsh, which changes the calling account.
	if user == "" {
		return runner.Sudo("chsh", "-s", shellBin).Run()
	}
	return runner.Sudo("chsh", "-s", shellBin, user).Run()
}

// LoginShellIs reports whether the target user's login shell is already the
// named shell, as setLoginShell would resolve it.
func LoginShellIs(name string) bool {
	shellBin := findShell(name)
	return shellBin != "" && currentLoginShell(targetUser()) == shellBin
}

// findShell returns the preferred install of the named shell: Homebrew's
// first, then the system's.
func findShell(name string) string {
	for _, dir := range []string{
		"/opt/homebrew/bin",
		"/home/linuxbrew/.linuxbrew/bin",
		"/usr/local/bin",
		"/bin",
		"/usr/bin",
	} {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// targetUser returns the human account whose shell should be changed, resolving
// the original user when running under sudo.
func targetUser() string {
	if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
		return u
	}
	return os.Getenv("USER")
}

// currentLoginShell returns user's configured login shell from the user
// database (Directory Service on macOS, passwd on Linux), or "" if unknown.
func currentLoginShell(user string) string {
	if user == "" {
		return ""
	}
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("dscl", ".", "-read", "/Users/"+user, "UserShell").Output()
		if err != nil {
			return ""
		}
		fields := strings.Fields(string(out)) // "UserShell: /bin/zsh"
		if len(fields) >= 2 {
			return fields[len(fields)-1]
		}
		return ""
	}
	out, err := exec.Command("getent", "passwd", user).Output()
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimSpace(string(out)), ":")
	if len(parts) >= 7 {
		return parts[6]
	}
	return ""
}
