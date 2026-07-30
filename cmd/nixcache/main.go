// Command nixcache stores this repository's Nix binary cache in GHCR.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:           "nixcache",
		Short:         "GHCR-backed Nix cache for standalone-binaries",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	push := &cobra.Command{
		Use:   "push <store-path>...",
		Short: "Publish store path closures to the cache",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRegistryClient(cacheRepository, false)
			if err != nil {
				return err
			}
			repoRoot, err := os.Getwd()
			if err != nil {
				return err
			}
			return pushPaths(client, repoRoot, args)
		},
	}

	serve := &cobra.Command{
		Use:   "serve",
		Short: "Serve the cache as a local Nix substituter",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := newRegistryClient(cacheRepository, false)
			if err != nil {
				return err
			}
			return serveCache(client)
		},
	}

	var keep int
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
			return pruneCache(client, keep, dryRun)
		},
	}
	prune.Flags().IntVar(&keep, "keep", 2, "number of most recent snapshots to keep per system")
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
			total, err := cacheSize(client)
			if err != nil {
				return err
			}
			fmt.Printf("%d\t%s\n", total, humanSize(total))
			return nil
		},
	}

	root.AddCommand(push, serve, prune, size)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
