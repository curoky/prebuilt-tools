package main

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const (
	loadDylib       = uint32(0xc)
	idDylib         = uint32(0xd)
	loadWeakDylib   = uint32(0x80000018)
	loadRpath       = uint32(0x8000001c)
	reexportDylib   = uint32(0x8000001f)
	lazyLoadDylib   = uint32(0x20)
	loadUpwardDylib = uint32(0x80000023)
)

type elfInfo struct {
	interpreter string
	libraries   []string
	rpaths      []string
	isDynamic   bool
}

type machOInfo struct {
	dependencies []string
	rpaths       []string
	hasNixLoad   bool
}

func isELFMagic(magic [4]byte) bool {
	return magic == [4]byte{0x7f, 'E', 'L', 'F'}
}

func isMachOMagic(magic [4]byte) bool {
	const (
		magicFat64 = uint32(0xcafebabf)
	)
	valueBE := binary.BigEndian.Uint32(magic[:])
	valueLE := binary.LittleEndian.Uint32(magic[:])
	return valueBE == macho.MagicFat ||
		valueLE == macho.MagicFat ||
		valueBE == magicFat64 ||
		valueLE == magicFat64 ||
		valueBE == macho.Magic32 ||
		valueBE == macho.Magic64 ||
		valueLE == macho.Magic32 ||
		valueLE == macho.Magic64
}

func validateELF(path, artifactName string, allowDynamic bool) error {
	info, err := inspectELF(path)
	if err != nil {
		return fmt.Errorf("inspect ELF %s: %w", path, err)
	}
	if info.isDynamic && !allowDynamic {
		return fmt.Errorf("%s is dynamically linked; Linux artifact %s must contain only statically linked ELF files", path, artifactName)
	}
	for _, value := range append(append([]string(nil), info.libraries...), info.rpaths...) {
		if strings.Contains(value, "/nix") {
			return fmt.Errorf("%s has a dynamic path under /nix: %s", path, value)
		}
	}
	if strings.Contains(info.interpreter, "/nix") {
		return fmt.Errorf("%s has an interpreter under /nix: %s", path, info.interpreter)
	}
	return nil
}

func inspectELF(path string) (elfInfo, error) {
	file, err := elf.Open(path)
	if err != nil {
		return elfInfo{}, err
	}
	defer file.Close()

	info := elfInfo{}
	for _, program := range file.Progs {
		switch program.Type {
		case elf.PT_INTERP:
			data, err := io.ReadAll(program.Open())
			if err != nil {
				return elfInfo{}, err
			}
			info.interpreter = strings.TrimRight(string(data), "\x00")
		}
	}
	info.libraries, err = file.ImportedLibraries()
	if err != nil {
		return elfInfo{}, err
	}
	flags, err := file.DynValue(elf.DT_FLAGS_1)
	if err != nil {
		return elfInfo{}, err
	}
	info.isDynamic = isDynamicallyLinkedELF(file.Type, info.interpreter, info.libraries, flags)
	for _, tag := range []elf.DynTag{elf.DT_RPATH, elf.DT_RUNPATH} {
		values, err := file.DynString(tag)
		if err != nil {
			return elfInfo{}, err
		}
		info.rpaths = append(info.rpaths, values...)
	}
	return info, nil
}

func isDynamicallyLinkedELF(fileType elf.Type, interpreter string, libraries []string, flags []uint64) bool {
	if interpreter != "" || len(libraries) > 0 {
		return true
	}
	if fileType != elf.ET_DYN {
		return false
	}
	for _, value := range flags {
		if elf.DynFlag1(value)&elf.DF_1_PIE != 0 {
			return false
		}
	}
	return true
}

func normalizeMachO(path string) (machOInfo, error) {
	info, err := inspectMachO(path)
	if err != nil {
		return machOInfo{}, fmt.Errorf("inspect Mach-O %s: %w", path, err)
	}
	seen := make(map[string]bool)
	for _, rpath := range info.rpaths {
		if !strings.HasPrefix(rpath, "/nix/") || seen[rpath] {
			continue
		}
		seen[rpath] = true
		command := exec.Command("install_name_tool", "-delete_rpath", rpath, path)
		if output, err := command.CombinedOutput(); err != nil {
			return machOInfo{}, fmt.Errorf("delete Mach-O rpath %s from %s: %w: %s", rpath, path, err, bytes.TrimSpace(output))
		}
	}
	if len(seen) > 0 {
		info, err = inspectMachO(path)
		if err != nil {
			return machOInfo{}, fmt.Errorf("reinspect Mach-O %s: %w", path, err)
		}
	}
	return info, nil
}

func validateMachO(path string, info machOInfo) error {
	for _, dependency := range info.dependencies {
		if !isPortableMachODependency(dependency) {
			return fmt.Errorf("%s has a non-portable dynamic dependency: %s", path, dependency)
		}
	}
	if info.hasNixLoad {
		return fmt.Errorf("%s has a Mach-O load command under /nix", path)
	}
	for _, rpath := range info.rpaths {
		if rpath != "@loader_path" && !strings.HasPrefix(rpath, "@loader_path/") {
			return fmt.Errorf("%s has a non-portable LC_RPATH: %s", path, rpath)
		}
	}
	return nil
}

func isPortableMachODependency(path string) bool {
	return strings.HasPrefix(path, "/usr/lib/") ||
		strings.HasPrefix(path, "/System/Library/Frameworks/") ||
		strings.HasPrefix(path, "@loader_path/") ||
		strings.HasPrefix(path, "@rpath/")
}

func inspectMachO(path string) (machOInfo, error) {
	fatFile, err := macho.OpenFat(path)
	if err == nil {
		defer fatFile.Close()
		var result machOInfo
		for _, architecture := range fatFile.Arches {
			result.merge(inspectMachOFile(architecture.File))
		}
		return result, nil
	}
	if !errors.Is(err, macho.ErrNotFat) {
		return machOInfo{}, err
	}

	file, err := macho.Open(path)
	if err != nil {
		return machOInfo{}, err
	}
	defer file.Close()
	return inspectMachOFile(file), nil
}

func inspectMachOFile(file *macho.File) machOInfo {
	var info machOInfo
	for _, load := range file.Loads {
		raw := load.Raw()
		if bytes.Contains(raw, []byte("/nix")) {
			info.hasNixLoad = true
		}
		if len(raw) < 12 {
			continue
		}
		command := file.ByteOrder.Uint32(raw[:4])
		offset := file.ByteOrder.Uint32(raw[8:12])
		name := loadCommandString(raw, offset)
		switch command {
		case loadDylib, loadWeakDylib, reexportDylib, lazyLoadDylib, loadUpwardDylib:
			if name != "" {
				info.dependencies = append(info.dependencies, name)
			}
		case idDylib:
		case loadRpath:
			if name != "" {
				info.rpaths = append(info.rpaths, name)
			}
		}
	}
	return info
}

func loadCommandString(raw []byte, offset uint32) string {
	if offset >= uint32(len(raw)) {
		return ""
	}
	value := raw[offset:]
	if index := bytes.IndexByte(value, 0); index >= 0 {
		value = value[:index]
	}
	return string(value)
}

func (info *machOInfo) merge(other machOInfo) {
	info.dependencies = append(info.dependencies, other.dependencies...)
	info.rpaths = append(info.rpaths, other.rpaths...)
	info.hasNixLoad = info.hasNixLoad || other.hasNixLoad
}
