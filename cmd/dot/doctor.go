package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AugustDG/dot/internal/bootstrap"
	"github.com/AugustDG/dot/internal/brew"
	"github.com/AugustDG/dot/internal/config"
	gitops "github.com/AugustDG/dot/internal/git"
	"github.com/AugustDG/dot/internal/platform"
	"github.com/AugustDG/dot/internal/stow"
	"github.com/AugustDG/dot/internal/tui"
	"github.com/spf13/cobra"
)

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the dotfiles setup and report problems",
		Long: "Checks required tools, repo state, PATH, module dependencies, submodules\n" +
			"and dangling links, plus GitHub auth and the login shell when dot.toml asks\n" +
			"for them. Exits non-zero if any check fails.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor()
		},
	}
}

func runDoctor() error {
	dotfilesDir := platform.DotfilesDir()
	homeDir := platform.HomeDir()

	checks := environmentChecks(dotfilesDir, homeDir)

	modules, err := config.DiscoverModules(dotfilesDir)
	if err != nil {
		checks = append(checks, tui.Check{Name: "modules", Level: tui.LevelFail, Detail: err.Error()})
	} else {
		checks = append(checks, moduleChecks(dotfilesDir, modules)...)
		checks = append(checks, danglingCheck(dotfilesDir, homeDir, modules))
	}

	fmt.Println(tui.RenderChecks("Health check", checks))

	fails := 0
	for _, c := range checks {
		if c.Level == tui.LevelFail {
			fails++
		}
	}
	if fails > 0 {
		return fmt.Errorf("%d check(s) failed", fails)
	}
	return nil
}

// environmentChecks covers tools, repo, auth, shell, and PATH.
func environmentChecks(dotfilesDir, homeDir string) []tui.Check {
	var checks []tui.Check

	checks = append(checks,
		toolCheck("git", true),
		toolCheck("stow", true),
		toolCheck("brew", false),
	)

	if _, err := os.Stat(filepath.Join(dotfilesDir, ".git")); err != nil {
		checks = append(checks, tui.Check{Name: "repo", Level: tui.LevelFail,
			Detail: dotfilesDir + " is not a git repository"})
	} else {
		checks = append(checks, repoStateCheck(dotfilesDir))
	}

	manifest, err := config.LoadManifest(dotfilesDir)
	if err != nil {
		checks = append(checks, tui.Check{Name: config.ManifestFile, Level: tui.LevelFail, Detail: err.Error()})
	}
	if manifest.Bootstrap.GitHubAuth {
		checks = append(checks, githubAuthCheck())
	}
	if manifest.Bootstrap.Shell != "" {
		checks = append(checks, shellCheck(manifest.Bootstrap.Shell))
	}

	return append(checks, localBinCheck())
}

func githubAuthCheck() tui.Check {
	if gitops.IsGHAuthenticated() {
		return tui.Check{Name: "github auth", Level: tui.LevelOK, Detail: "authenticated"}
	}
	return tui.Check{Name: "github auth", Level: tui.LevelWarn, Detail: "not logged in (run: gh auth login)"}
}

func toolCheck(name string, required bool) tui.Check {
	if path, err := exec.LookPath(name); err == nil {
		return tui.Check{Name: name, Level: tui.LevelOK, Detail: path}
	}
	level := tui.LevelWarn
	if required {
		level = tui.LevelFail
	}
	return tui.Check{Name: name, Level: level, Detail: "not found on PATH"}
}

func repoStateCheck(dotfilesDir string) tui.Check {
	branch, err := gitops.CurrentBranch(dotfilesDir)
	if err != nil {
		return tui.Check{Name: "repo state", Level: tui.LevelWarn, Detail: "detached HEAD"}
	}
	ahead, behind := gitops.AheadBehind(dotfilesDir)
	dirty := gitops.IsDirty(dotfilesDir)

	var notes []string
	if dirty {
		notes = append(notes, "uncommitted changes")
	}
	if !gitops.HasUpstream(dotfilesDir) {
		notes = append(notes, "no upstream tracking branch")
	}
	if ahead > 0 {
		notes = append(notes, fmt.Sprintf("%d unpushed", ahead))
	}
	if behind > 0 {
		notes = append(notes, fmt.Sprintf("%d behind", behind))
	}
	if len(notes) == 0 {
		return tui.Check{Name: "repo state", Level: tui.LevelOK, Detail: branch + ", clean and pushed"}
	}
	return tui.Check{Name: "repo state", Level: tui.LevelWarn,
		Detail: branch + ": " + strings.Join(notes, ", ")}
}

func shellCheck(name string) tui.Check {
	if bootstrap.LoginShellIs(name) {
		return tui.Check{Name: "login shell", Level: tui.LevelOK, Detail: name}
	}
	return tui.Check{Name: "login shell", Level: tui.LevelWarn,
		Detail: "not " + name + " (run: dot init)"}
}

func localBinCheck() tui.Check {
	bin := platform.LocalBin()
	if platform.PathContains(bin) {
		return tui.Check{Name: "~/.local/bin", Level: tui.LevelOK, Detail: "on PATH"}
	}
	return tui.Check{Name: "~/.local/bin", Level: tui.LevelWarn, Detail: "not on PATH"}
}

// moduleChecks reports, for each installed module, its dependency and submodule
// health. Modules that aren't stowed are summarised in a single line.
func moduleChecks(dotfilesDir string, modules []config.Module) []tui.Check {
	var formulae, casks map[string]bool
	if brew.IsInstalled() {
		formulae = brew.InstalledFormulae()
		casks = brew.InstalledCasks()
	}

	var checks []tui.Check
	notInstalled := 0
	for _, mod := range modules {
		if !mod.SupportsOS(platform.DetectOS()) {
			continue
		}
		if !mod.IsStowed {
			notInstalled++
			continue
		}

		var notes []string
		level := tui.LevelOK

		if formulae != nil {
			if missing := bootstrap.MissingDeps(mod.Deps, formulae, casks); len(missing) > 0 {
				notes = append(notes, "missing deps: "+strings.Join(missing, ", "))
				level = tui.LevelWarn
			}
		}
		if mod.HasSubmodule && len(mod.SubmodulePaths) > 0 {
			state, _ := gitops.SubmoduleStatus(dotfilesDir, mod.SubmodulePaths[0])
			switch state {
			case "not-init":
				notes = append(notes, "submodule not initialised")
				level = tui.LevelWarn
			case "dirty":
				notes = append(notes, "submodule has local changes")
				if level == tui.LevelOK {
					level = tui.LevelWarn
				}
			}
		}

		detail := "stowed"
		if len(notes) > 0 {
			detail = strings.Join(notes, "; ")
		}
		checks = append(checks, tui.Check{Name: "module " + mod.Name, Level: level, Detail: detail})
	}

	if notInstalled > 0 {
		checks = append(checks, tui.Check{Name: "modules", Level: tui.LevelOK,
			Detail: fmt.Sprintf("%d not installed (skipped)", notInstalled)})
	}
	return checks
}

func danglingCheck(dotfilesDir, homeDir string, modules []config.Module) tui.Check {
	roots := stow.ScanRoots(dotfilesDir, homeDir, moduleNames(modules))
	dangling := stow.FindDangling(dotfilesDir, homeDir, roots)
	if len(dangling) == 0 {
		return tui.Check{Name: "dangling links", Level: tui.LevelOK, Detail: "none"}
	}
	return tui.Check{Name: "dangling links", Level: tui.LevelWarn,
		Detail: fmt.Sprintf("%d found (run: dot clean)", len(dangling))}
}
