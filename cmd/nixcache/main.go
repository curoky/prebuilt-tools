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

	root.AddCommand(push, serve)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
