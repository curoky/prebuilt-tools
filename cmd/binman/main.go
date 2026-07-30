// Command bm installs standalone binaries published as OCI artifacts.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/spf13/cobra"
)

const (
	defaultRegistry = "ghcr.io/curoky/standalone-binaries"
	metaFile        = ".binman-meta"
	defaultPrefix   = "/opt/binman"
	logFile         = "binman.log"
	maxParallel     = 16 // cap concurrent registry requests / downloads
)

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var logger = slog.New(slog.NewTextHandler(io.Discard, nil))

func setupLogger(prefix string, verbose bool) (io.Closer, error) {
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create prefix %s: %w", prefix, err)
	}
	path := filepath.Join(prefix, logFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("cannot open log file %s: %w", path, err)
	}
	var w io.Writer = f
	if verbose {
		w = io.MultiWriter(f, os.Stderr)
	}
	logger = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return f, nil
}

func detectArch() (string, error) {
	switch {
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return "linux-x86_64", nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return "darwin-arm64", nil
	}
	return "", fmt.Errorf("unsupported platform %s/%s; pass --arch linux-x86_64 or darwin-arm64",
		runtime.GOOS, runtime.GOARCH)
}

func validateArch(arch string) error {
	if arch != "linux-x86_64" && arch != "darwin-arm64" {
		return fmt.Errorf("unsupported arch %q; expected linux-x86_64 or darwin-arm64", arch)
	}
	return nil
}

func validateName(kind, name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid %s name %q", kind, name)
	}
	return nil
}

func validatePackageName(name string) error { return validateName("package", name) }
func validateProfileName(name string) error { return validateName("profile", name) }

func main() {
	var (
		prefix  string
		arch    string
		link    bool
		force   bool
		verbose bool
	)
	var logCloser io.Closer

	resolveArch := func() (string, error) {
		if arch != "" {
			return arch, nil
		}
		return detectArch()
	}

	root := &cobra.Command{
		Use:           "bm",
		Short:         "package manager for ghcr.io/curoky/standalone-binaries",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			c, err := setupLogger(prefix, verbose)
			if err != nil {
				return err
			}
			logCloser = c
			logger.Info("bm invoked", "command", cmd.Name(), "args", args,
				"prefix", prefix, "arch", arch, "verbose", verbose)
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if logCloser != nil {
				_ = logCloser.Close()
			}
		},
	}
	pf := root.PersistentFlags()
	pf.StringVar(&prefix, "prefix", defaultPrefix, "install prefix")
	pf.StringVar(&arch, "arch", "", "arch tag: linux-x86_64 | darwin-arm64 (auto-detected)")
	pf.BoolVar(&verbose, "verbose", false, "also print the detailed log to stderr")

	install := &cobra.Command{
		Use:   "install <package>...",
		Short: "Install/refresh one or more packages",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := resolveArch()
			if err != nil {
				return err
			}
			return installPackages(args, installOpts{prefix: prefix, arch: a, linked: link, force: force})
		},
	}
	install.Flags().BoolVar(&link, "link", true, "expose binaries via relative symlinks")
	install.Flags().BoolVar(&force, "force", false, "reinstall even if the digest already matches")

	remove := &cobra.Command{
		Use:   "remove <package>",
		Short: "Uninstall a package and clean up its links",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return cmdRemove(prefix, args[0]) },
	}

	upgrade := &cobra.Command{
		Use:   "upgrade [package...]",
		Short: "Upgrade the given packages, or all installed packages if none is given",
		RunE: func(cmd *cobra.Command, args []string) error {
			overrideArch := ""
			if cmd.Flags().Changed("arch") {
				overrideArch = arch
			}
			return cmdUpgrade(prefix, overrideArch, args)
		},
	}

	info := &cobra.Command{
		Use:   "info <package>",
		Short: "Show a package's metadata and whether it is up to date",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := resolveArch()
			if err != nil {
				return err
			}
			return cmdInfo(prefix, a, args[0])
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List installed packages and their recorded digests",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmdList(prefix) },
	}

	outdated := &cobra.Command{
		Use:   "outdated",
		Short: "Show installed packages whose remote digest has changed",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmdOutdated(prefix) },
	}

	var prune bool
	sync := &cobra.Command{
		Use:   "sync [file]",
		Short: "Install packages declared in a YAML manifest (default: binman.yaml)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := resolveArch()
			if err != nil {
				return err
			}
			file := defaultManifest
			if len(args) == 1 {
				file = args[0]
			}
			return cmdSync(prefix, a, file,
				cmd.Flags().Changed("prefix"), cmd.Flags().Changed("arch"), force, prune)
		},
	}
	sync.Flags().BoolVar(&force, "force", false, "reinstall even if the digest already matches")
	sync.Flags().BoolVar(&prune, "prune", false, "remove installed packages not listed in the manifest")

	root.AddCommand(install, remove, upgrade, info, list, outdated, sync)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
