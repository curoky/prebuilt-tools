package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

type platform string

const (
	platformLinux  platform = "linux"
	platformDarwin platform = "darwin"
)

type config struct {
	source          string
	output          string
	archive         string
	name            string
	platform        platform
	allowDynamicELF bool
}

func main() {
	var cfg config
	var targetPlatform string

	flag.StringVar(&cfg.source, "source", "", "source package tree")
	flag.StringVar(&cfg.output, "output", "", "standalone output directory")
	flag.StringVar(&cfg.archive, "archive", "", "output tar.gz")
	flag.StringVar(&cfg.name, "name", "", "artifact name")
	flag.StringVar(&targetPlatform, "platform", "", "target platform: linux or darwin")
	flag.BoolVar(&cfg.allowDynamicELF, "allow-dynamic-elf", false, "allow dynamically linked ELF files")
	flag.Parse()

	cfg.platform = platform(targetPlatform)
	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "artifact:", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	if cfg.source == "" || cfg.output == "" || cfg.archive == "" || cfg.name == "" {
		return errors.New("source, output, archive, and name are required")
	}
	if cfg.platform != platformLinux && cfg.platform != platformDarwin {
		return fmt.Errorf("unsupported platform %q", cfg.platform)
	}
	archiveName, err := cleanArchiveName(cfg.name)
	if err != nil {
		return err
	}
	cfg.name = archiveName
	if err := normalize(cfg); err != nil {
		return err
	}
	if err := createArchive(cfg.output, cfg.archive, cfg.name); err != nil {
		return fmt.Errorf("create archive: %w", err)
	}
	return nil
}
