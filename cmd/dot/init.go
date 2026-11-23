package main

import (
	"fmt"
	"path/filepath"

	"github.com/AugustDG/dot/internal/bootstrap"
	"github.com/AugustDG/dot/internal/config"
	gitops "github.com/AugustDG/dot/internal/git"
	"github.com/AugustDG/dot/internal/platform"
	"github.com/AugustDG/dot/internal/tui"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var (
		adopt bool
		dir   string
	)

	cmd := &cobra.Command{
		Use:   "init [repo]",
		Short: "Set up this machine from a dotfiles repo, then link modules",
		Long: "Installs Homebrew, git and stow, then clones repo (a URL, a local path, or\n" +
			"GitHub owner/name) into --dir. With no repo and nothing at --dir, it starts\n" +
			"an empty repo there. The dir is saved to " + platform.ConfigFile() + "\n" +
			"so later commands find it.\n\n" +
			"Next it applies the repo's dot.toml (brew packages, GitHub auth, login\n" +
			"shell, backups) and links modules: a terminal gets a picker; without one,\n" +
			"every OS-compatible module is linked. Safe to rerun.",
		Example: "  dot init AugustDG/dotfiles\n" +
			"  dot init git@github.com:you/dotfiles.git --dir ~/src/dotfiles\n" +
			"  dot init                     # start an empty repo at ~/.dotfiles",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dotfilesDir := platform.DotfilesDir()
			if dir != "" {
				abs, err := filepath.Abs(platform.ExpandHome(dir))
				if err != nil {
					return err
				}
				dotfilesDir = abs
			}
			repoURL := ""
			if len(args) == 1 {
				repoURL = gitops.ResolveRepoURL(args[0])
			}

			inst := bootstrap.NewInstaller(dotfilesDir)
			if err := runFatalTasks("Setting up", inst.SetupTasks(repoURL)); err != nil {
				return err
			}

			manifest, err := config.LoadManifest(dotfilesDir)
			if err != nil {
				return fmt.Errorf("read %s: %w", config.ManifestPath(dotfilesDir), err)
			}
			if tasks := inst.ConfigureTasks(manifest.Bootstrap); len(tasks) > 0 {
				if err := runFatalTasks("Applying "+config.ManifestFile, tasks); err != nil {
					return err
				}
			}

			return runLink(dotfilesDir, linkOptions{all: !platform.IsInteractive(), adopt: adopt}, nil)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "Where to put the repo (default: the saved dir, or ~/.dotfiles)")
	cmd.Flags().BoolVar(&adopt, "adopt", false, "On conflict, absorb existing target files into the repo (stow --adopt), then symlink")
	return cmd
}

// runFatalTasks runs tasks under a heading and returns the first error from
// a task marked Fatal.
func runFatalTasks(heading string, tasks []tui.Task) error {
	errs, err := tui.RunTasks(heading, tasks)
	if err != nil {
		return err
	}
	if err := tui.FatalError(tasks, errs); err != nil {
		return fmt.Errorf("%s failed: %w", heading, err)
	}
	fmt.Println()
	return nil
}
