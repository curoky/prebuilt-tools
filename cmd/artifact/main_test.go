package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRunNormalizesTreeAndCreatesArchive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "source")
	output := filepath.Join(root, "output")
	archive := filepath.Join(root, "artifact.tar.gz")
	mustMkdirAll(t, filepath.Join(source, "bin"))
	mustMkdirAll(t, filepath.Join(source, "share", "doc"))
	mustWriteFile(t, filepath.Join(source, ".hidden"), "hidden\n", 0o444)
	mustWriteFile(t, filepath.Join(source, "empty"), "", 0o444)
	mustWriteFile(t, filepath.Join(source, "share", "doc", "removed"), "doc\n", 0o444)
	mustWriteFile(t, filepath.Join(source, "bin", "tool"), "#!/nix/store/0123456789abcdfghijklmnpqrsvwxyz-bash/bin/bash\n/nix/store/0123456789abcdfghijklmnpqrsvwxyz-coreutils/bin/echo\n", 0o555)
	mustWriteFile(t, filepath.Join(source, "bin", "wrapped"), "launcher\n", 0o555)
	mustWriteFile(t, filepath.Join(source, "bin", ".wrapped-wrapped"), "#!/nix/store/0123456789abcdfghijklmnpqrsvwxyz-bash/bin/bash\nwrapped\n", 0o555)
	mustWriteFile(t, filepath.Join(source, "lib.a"), "archive\n", 0o444)
	if err := os.Symlink("lib.a", filepath.Join(source, "lib-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tool", filepath.Join(source, "bin", "tool-link")); err != nil {
		t.Fatal(err)
	}

	external := filepath.Join(root, "external")
	mustMkdirAll(t, external)
	mustWriteFile(t, filepath.Join(external, "data"), "external\n", 0o444)
	if err := os.Symlink(external, filepath.Join(source, "external-data")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(source, "bin", "tool"), filepath.Join(source, "absolute-tool")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(source, "dangling")); err != nil {
		t.Fatal(err)
	}

	err := run(config{
		source:   source,
		output:   output,
		archive:  archive,
		name:     "fixture",
		platform: platformLinux,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(output, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				_ = os.Chmod(path, 0o755)
			}
			return nil
		})
	})

	assertExists(t, filepath.Join(output, ".hidden"))
	assertAbsent(t, filepath.Join(output, "share", "doc"))
	assertAbsent(t, filepath.Join(output, "lib.a"))
	assertAbsent(t, filepath.Join(output, "lib-link"))
	assertAbsent(t, filepath.Join(output, "dangling"))
	assertRegularFile(t, filepath.Join(output, "absolute-tool"))
	assertRegularFile(t, filepath.Join(output, "external-data", "data"))
	if target, err := os.Readlink(filepath.Join(output, "bin", "tool-link")); err != nil || target != "tool" {
		t.Fatalf("internal symlink target=%q err=%v", target, err)
	}
	assertExists(t, filepath.Join(output, "bin", "wrapped"))
	assertAbsent(t, filepath.Join(output, "bin", ".wrapped-wrapped"))
	wrapped, err := os.ReadFile(filepath.Join(output, "bin", "wrapped"))
	if err != nil {
		t.Fatal(err)
	}
	if string(wrapped) != "#!/usr/bin/env bash\nwrapped\n" {
		t.Fatalf("wrapped binary was not normalized after replacing launcher: %q", wrapped)
	}

	script, err := os.ReadFile(filepath.Join(output, "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(script), "/nix/store") || !strings.HasPrefix(string(script), "#!/usr/bin/env bash\n") {
		t.Fatalf("unexpected rewritten script:\n%s", script)
	}

	entries := readArchive(t, archive)
	wantEntries := []string{
		"fixture/",
		"fixture/.hidden",
		"fixture/absolute-tool",
		"fixture/bin/",
		"fixture/bin/tool",
		"fixture/bin/tool-link",
		"fixture/bin/wrapped",
		"fixture/empty",
		"fixture/external-data/",
		"fixture/external-data/data",
		"fixture/share/",
	}
	if !reflect.DeepEqual(entries, wantEntries) {
		t.Fatalf("archive entries=%q want=%q", entries, wantEntries)
	}
	modes := readArchiveModes(t, archive)
	if modes["fixture/"] != 0o755 || modes["fixture/.hidden"] != 0o644 || modes["fixture/bin/tool"] != 0o755 {
		t.Fatalf("archive modes=%v", modes)
	}
}

func TestArchiveIsDeterministic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mustMkdirAll(t, tree)
	mustWriteFile(t, filepath.Join(tree, "file"), "data\n", 0o444)
	first := filepath.Join(root, "first.tar.gz")
	second := filepath.Join(root, "second.tar.gz")
	if err := createArchive(tree, first, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(tree, "file"), archiveTime.AddDate(1, 0, 0), archiveTime.AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := createArchive(tree, second, "fixture"); err != nil {
		t.Fatal(err)
	}

	if fileHash(t, first) != fileHash(t, second) {
		t.Fatal("archive changed when only source mtime changed")
	}
}

func TestNukeStoreReferences(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "binary")
	mustWriteFile(t, path, "\x00/nix/store/0123456789abcdfghijklmnpqrsvwxyz-package/bin\x00", 0o555)
	if err := nukeStoreReferences(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/nix/store/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee-package/") {
		t.Fatalf("store reference was not nuked: %q", data)
	}
}

func TestValidateELF(t *testing.T) {
	t.Parallel()

	staticBinary := currentExecutable(t)
	if _, err := inspectELF(staticBinary); err != nil {
		t.Skipf("test binary is not ELF on this platform: %v", err)
	}
	if err := validateELF(staticBinary, "fixture", false); err != nil {
		t.Fatalf("current test binary should be static: %v", err)
	}

	dynamicBinary := "/bin/true"
	info, err := inspectELF(dynamicBinary)
	if err != nil {
		t.Skipf("host /bin/true is not ELF: %v", err)
	}
	if info.interpreter == "" && len(info.libraries) == 0 {
		t.Skip("host /bin/true is static")
	}
	if err := validateELF(dynamicBinary, "fixture", false); err == nil || !strings.Contains(err.Error(), "dynamically linked") {
		t.Fatalf("dynamic ELF error=%v", err)
	}
	if err := validateELF(dynamicBinary, "fixture", true); err != nil {
		t.Fatalf("allowed dynamic ELF failed: %v", err)
	}
}

func TestDynamicallyLinkedELFClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fileType    elf.Type
		interpreter string
		libraries   []string
		flags       []uint64
		want        bool
	}{
		{name: "static executable", fileType: elf.ET_EXEC},
		{name: "static PIE", fileType: elf.ET_DYN, flags: []uint64{uint64(elf.DF_1_PIE)}},
		{name: "dynamic PIE", fileType: elf.ET_DYN, interpreter: "/lib/ld-musl.so.1", flags: []uint64{uint64(elf.DF_1_PIE)}, want: true},
		{name: "shared object", fileType: elf.ET_DYN, want: true},
		{name: "needed library", fileType: elf.ET_EXEC, libraries: []string{"libc.so.6"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isDynamicallyLinkedELF(test.fileType, test.interpreter, test.libraries, test.flags)
			if got != test.want {
				t.Fatalf("got %t, want %t", got, test.want)
			}
		})
	}
}

func TestValidateMachO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		info    machOInfo
		wantErr string
	}{
		{
			name: "portable",
			info: machOInfo{
				dependencies: []string{"/usr/lib/libSystem.B.dylib", "@loader_path/libhelper.dylib"},
				rpaths:       []string{"@loader_path/../lib"},
			},
		},
		{
			name:    "absolute dependency",
			info:    machOInfo{dependencies: []string{"/opt/homebrew/lib/libhelper.dylib"}},
			wantErr: "non-portable dynamic dependency",
		},
		{
			name:    "nix load command",
			info:    machOInfo{hasNixLoad: true},
			wantErr: "load command under /nix",
		},
		{
			name:    "absolute rpath",
			info:    machOInfo{rpaths: []string{"/opt/homebrew/lib"}},
			wantErr: "non-portable LC_RPATH",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateMachO("fixture", test.info)
			if test.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("error=%v want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestLoadCommandString(t *testing.T) {
	t.Parallel()

	raw := make([]byte, 64)
	binary.LittleEndian.PutUint32(raw[:4], loadDylib)
	binary.LittleEndian.PutUint32(raw[8:12], 24)
	copy(raw[24:], "@loader_path/My Library.dylib")
	if got := loadCommandString(raw, 24); got != "@loader_path/My Library.dylib" {
		t.Fatalf("got %q", got)
	}
}

func TestMachOMagic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value [8]byte
		want  bool
	}{
		{name: "fat big endian", value: [8]byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 2}, want: true},
		{name: "fat little endian", value: [8]byte{0xbe, 0xba, 0xfe, 0xca, 0, 0, 0, 0}, want: true},
		{name: "fat64 big endian", value: [8]byte{0xca, 0xfe, 0xba, 0xbf, 0, 0, 0, 0}, want: true},
		{name: "fat64 little endian", value: [8]byte{0xbf, 0xba, 0xfe, 0xca, 0, 0, 0, 0}, want: true},
		{name: "64-bit big endian", value: [8]byte{0xfe, 0xed, 0xfa, 0xcf, 0, 0, 0, 0}, want: true},
		{name: "64-bit little endian", value: [8]byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0, 0, 0}, want: true},
		{name: "java class file", value: [8]byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 52}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isMachOMagic(test.value); got != test.want {
				t.Fatalf("isMachOMagic(%x) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestInspectELFReadsInterpreter(t *testing.T) {
	t.Parallel()

	path := "/bin/true"
	file, err := elf.Open(path)
	if err != nil {
		t.Skipf("%s is not ELF: %v", path, err)
	}
	file.Close()
	info, err := inspectELF(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.interpreter == "" {
		t.Skip("host /bin/true has no ELF interpreter")
	}
}

func currentExecutable(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s still exists: %v", path, err)
	}
}

func assertRegularFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("%s mode=%s, want regular file", path, info.Mode())
	}
}

func readArchive(t *testing.T, path string) []string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)

	var entries []string
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, header.Name)
	}
	return entries
}

func readArchiveModes(t *testing.T, path string) map[string]int64 {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)

	modes := make(map[string]int64)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		modes[header.Name] = header.Mode
	}
	return modes
}

func fileHash(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(data)
}
