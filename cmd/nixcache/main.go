// Command nixcache stores this repository's Nix binary cache in GHCR.
package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	os.Exit(run())
}

func run() int {
	root := &cobra.Command{
		Use:           "nixcache",
		Short:         "GHCR-backed Nix cache for standalone-binaries",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var packageKey string
	push := &cobra.Command{
		Use:   "push <store-path>...",
		Short: "Publish store path closures to the cache",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if packageKey == "" {
				packageKey = os.Getenv("NIXCACHE_PACKAGE_KEY")
			}
			if packageKey == "" {
				return fmt.Errorf("package key is required via --key or NIXCACHE_PACKAGE_KEY")
			}
			client, err := newRegistryClient(cacheRepository, false)
			if err != nil {
				return err
			}
			repoRoot, err := os.Getwd()
			if err != nil {
				return err
			}
			return pushPaths(cmd.Context(), client, repoRoot, packageKey, args)
		},
	}
	push.Flags().StringVar(&packageKey, "key", "", "stable package identity for retention")

	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve the cache as a local Nix substituter",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := os.Getwd()
			if err != nil {
				return err
			}
			state, err := loadSnapshot(repoRoot)
			if err != nil {
				return fmt.Errorf("load cache snapshot: %w", err)
			}
			client, err := newRegistryClient(cacheRepository, false)
			if err != nil {
				return err
			}
			return serveCache(cmd.Context(), client, segmentTagPrefix(state.ID, state.System))
		},
	}

	var cacheURL string
	probe := &cobra.Command{
		Use:   "probe <store-path>",
		Short: "Check whether an exact store path exists in the local cache",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hit, err := probeStorePath(cmd.Context(), http.DefaultClient, cacheURL, args[0])
			if err != nil {
				return &probeCommandError{err: err}
			}
			if !hit {
				return errProbeMiss
			}
			return nil
		},
	}
	probe.Flags().StringVar(&cacheURL, "cache", "http://127.0.0.1:37515", "local cache URL")

	var keep int
	var packageRetainDays int
	var packageKeep int
	var dryRun bool
	prune := &cobra.Command{
		Use:   "prune",
		Short: "Delete cache segments of old snapshots, keeping the newest per system",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRegistryClient(cacheRepository, false)
			if err != nil {
				return err
			}
			return pruneCache(cmd.Context(), client, keep, packageRetainDays, packageKeep, dryRun)
		},
	}
	prune.Flags().IntVar(&keep, "snapshot-keep", 2, "number of most recent snapshots to keep per system")
	prune.Flags().IntVar(&packageRetainDays, "package-retain-days", 2, "within a kept snapshot, keep every segment pushed within this many days of each package's newest segment")
	prune.Flags().IntVar(&packageKeep, "package-keep", 2, "within a kept snapshot, keep at least this many newest segments per package when the day window holds fewer")
	prune.Flags().BoolVar(&dryRun, "dry-run", false, "list segments that would be deleted without deleting")

	size := &cobra.Command{
		Use:   "size",
		Short: "Print the deduplicated total size of the cache",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRegistryClient(cacheRepository, false)
			if err != nil {
				return err
			}
			total, err := cacheSize(cmd.Context(), client)
			if err != nil {
				return err
			}
			fmt.Printf("%d\t%s\n", total, humanSize(total))
			return nil
		},
	}

	root.AddCommand(push, serve, probe, prune, size)
	if err := root.Execute(); err != nil {
		if errors.Is(err, errProbeMiss) {
			return 1
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		var probeErr *probeCommandError
		if errors.As(err, &probeErr) {
			return probeErrorExitCode
		}
		return 1
	}
	return 0
}

var errProbeMiss = errors.New("cache miss")

type probeCommandError struct{ err error }

func (err *probeCommandError) Error() string { return err.err.Error() }
func (err *probeCommandError) Unwrap() error { return err.err }
