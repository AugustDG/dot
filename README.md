# dot

A CLI for dotfiles kept in a git repo as [GNU stow](https://www.gnu.org/software/stow/)
packages. It sets up a new machine from your repo, links modules into `$HOME`,
installs their dependencies, and commits and pushes your changes. It runs on
macOS and Linux.

A module is any top-level directory in the repo with a `module.toml`. Its
contents mirror `$HOME`, so `nvim/.config/nvim/init.lua` links to
`~/.config/nvim/init.lua`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/AugustDG/dot/main/install.sh | bash -s -- owner/dotfiles
```

This puts `dot` in `~/.local/bin` and runs `dot init owner/dotfiles`. That
installs Homebrew, git and stow, clones the repo to `~/.dotfiles`, applies its
`dot.toml`, and opens a picker to choose modules to link. Leave off
`-s -- owner/dotfiles` to start with an empty repo instead.

`DOT_INSTALL_DIR` changes where the binary goes. `DOT_SKIP_INIT=1` installs it
without running `init`.

Graphviz also ships a `dot` command. If you have it, put `~/.local/bin` before
the Homebrew and system bin dirs on your PATH, or install `dot` elsewhere with
`DOT_INSTALL_DIR`.

## Try it without a repo

```bash
dot init                          # empty repo at ~/.dotfiles, with a commented dot.toml
dot adopt zsh ~/.zshrc ~/.zshenv  # move them into a zsh module and link them back
dot status                        # what's linked, what's changed
cd ~/.dotfiles && git remote add origin git@github.com:you/dotfiles.git
dot sync -m "add zsh"             # commit and push
```

On another machine, `dot init you/dotfiles` brings it all back.

## Where the repo lives

`dot init` clones into `--dir` (default `~/.dotfiles`) and saves that path to
`~/.config/dot/config.toml`. Every other command reads the saved path.
`DOT_DIR` overrides it for one shell.

`init` accepts GitHub `owner/name` shorthand (cloned over HTTPS), any git URL,
or a local path.

## Commands

Run `dot <command> --help` for flags. `-v` shows the output of the underlying
git, stow and brew commands.

### Set up this machine

```bash
dot init owner/dotfiles            # Install brew/git/stow, clone, apply dot.toml, pick modules
dot init --adopt                   # Absorb conflicting $HOME files into the repo instead of failing
dot doctor                         # Check tools, repo, PATH, module deps, submodules, dangling links
dot self-update                    # Replace the binary with the latest release
```

`init` is safe to rerun. Without a terminal it links every module that
supports the current OS.

### Link modules into $HOME

```bash
dot link                           # Interactive picker
dot link nvim tmux                 # Stow modules, install their deps, run their hooks
dot link --all                     # Every OS-compatible module
dot unlink nvim                    # Remove its symlinks; the module stays in the repo
dot status                         # Repo state and a per-module table
dot status --check                 # Exit non-zero if dirty, unpushed or links are broken
dot deps [modules...]              # Install missing module dependencies
dot clean [--dry-run]              # Remove symlinks left by deleted files
```

### Sync the repo

```bash
dot pull [modules...]              # git pull, update submodules, re-stow linked modules
dot bump [modules...]              # Move submodules to their upstream HEAD, re-stow
dot sync [modules...] -m "msg"     # Commit and push, submodules first
dot sync --dry-run                 # Show what would be committed and pushed
```

`pull` checks out the submodule commits the repo records. `bump` moves
submodules to their own latest commit, which you then record with `sync`.

### Author modules

```bash
dot add fish --desc "Fish shell"   # Create fish/module.toml
dot adopt fish ~/.config/fish      # Move existing config into the module and link it
dot eject fish                     # Stop managing it; its files stay in $HOME as real files
dot edit [module]                  # Open the repo or a module in $EDITOR
```

## `module.toml`

```toml
name = "nvim"
description = "Neovim config"
os = ["darwin", "linux"]          # omit to support every OS

[deps]
brew = ["neovim"]                  # Homebrew formulae (macOS and Linuxbrew)
cask = ["font-hack-nerd-font"]     # Homebrew casks (macOS only)
apt  = ["build-essential"]         # apt packages (Debian, Ubuntu)
dnf  = ["gcc"]                     # dnf packages (Fedora, RHEL)

[hooks]
post_install = "nvim --headless '+Lazy! sync' +qa"
```

`link` installs the deps that apply to the current OS and skips any already
installed. The `post_install` hook runs in bash after each `link`, so write it
to be safe to rerun. Use hooks to fetch plugin managers, download binaries, or
create machine-local files that stay out of git.

A module can hold git submodules (for example, a `nvim/.config/nvim` that is
its own repo). `link` initialises them, and `sync` commits and pushes them
before the parent repo.

## `dot.toml`

An optional file at the repo root. `init` applies it after cloning. Every key
is optional:

```toml
[bootstrap]
brew = ["fzf", "ripgrep", "zsh"]   # Homebrew formulae for the whole setup
backup_targets = [".zshrc"]        # $HOME files moved to ~/.dot-backup-<time>/ before linking
shell = "zsh"                      # login shell to set (found in Homebrew or /bin, /usr/bin)
github_auth = true                 # run `gh auth login` if gh isn't logged in
```

`doctor` checks the login shell and GitHub auth only when `dot.toml` asks for
them.

## Example repo

[AugustDG/dotfiles](https://github.com/AugustDG/dotfiles) is a full setup
with zsh, nvim, tmux and git modules, submodules and hooks.

## Shell completion

```bash
dot completion zsh > "${fpath[1]}/_dot"
dot completion bash > ~/.local/share/bash-completion/completions/dot
```

Module names complete from the repo.

## Development

```bash
go build -o dot ./cmd/dot
go test ./...
```

Every push to `main` publishes binaries for darwin and linux (amd64, arm64) to
a rolling `latest` release. Pushing a `v*` tag makes a versioned release.
