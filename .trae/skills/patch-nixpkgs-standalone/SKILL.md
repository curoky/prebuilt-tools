---
name: "patch-nixpkgs-standalone"
description: "Patch a nixpkgs package into a portable standalone build with no /nix dynamic-lib dependency. Invoke when adding/fixing a package under packages/ or manifests/, or when a build retains /nix/store dynamic libraries. Splits guidance by Linux (force musl static) and macOS (only system libs may stay dynamic)."
---

# Patch a nixpkgs Package for Standalone Portability

Use this when adding or fixing a package in this repo so the shipped artifact is
portable. Read `CLAUDE.md` first — it is the source of truth; this skill is the
actionable procedure derived from it.

## Prime directive: patch ONLY when the stock static build fails

**Never patch a package that already builds and links correctly.** A local
patched derivation under `packages/` is justified **only** when the plain
upstream static build (`nixpkgs.pkgsStatic.<x>`) cannot compile/link. If
`pkgsStatic.<x>` builds cleanly and meets the platform goal, use it directly via
the manifest (`isStatic = true`) — do not add a local package. Every patch route
below is a fallback for a *specific* failure of the stock static build; prefer
the least invasive one, and prefer no patch at all.

## The Hard Rule (both platforms)

The shipped payload **must not depend on any dynamic library under `/nix`**.
Text-only packages (scripts, fonts, data, pure-Perl/Python source with no
compiled objects) are exempt from the static-linking goal — they only go through
`normalize.sh` shebang/path rewriting. Everything with an ELF/Mach-O binary must
meet the platform goal below.

## Step 0 — Decide the split

A package usually needs different treatment per platform. Wire platform-specific
derivations via `manifests/default.nix` per-platform keys or via
`packages/local.nix` (`linux` / `darwin` sets). Keep a shared file when the same
build works everywhere; split into `default.nix` (Linux) + `darwin.nix` when not.

## Step 1 — Is it text-only?

If the package ships **no** compiled binaries (only scripts / data / source):
- No static linking required.
- Ensure `normalize.sh` handles it (shebang rewrite, strip `/nix` path fragments).
- If it's a script needing a runtime (perl/python/node), use the **sibling-wrapper**
  pattern: rename `bin/tool` → `bin/_tool` and drop a wrapper that invokes the
  co-located static runtime sibling (`$store/perl/bin/perl`, `$store/python311/...`,
  `$store/nodejs-slim26/bin/node`). See `packages/exiftool`, `packages/cloc`.
- Done. Skip the rest.

---

## LINUX — Goal: force musl full static

Target: a fully static ELF (musl). **This does not require literally writing
`pkgsStatic.<x>` in the manifest** — it means the *result* is statically linked.
Try the routes in order; **do not patch if an earlier route already builds**:

1. **`isStatic = true` (default) in the manifest** — build straight from the
   `pkgsStatic` env, no local package. **If `pkgsStatic.<x>` compiles and links,
   STOP HERE — do not patch.** This is the default and works for most tools. See
   `packages/wget/default.nix` (`pkgsStatic.wget` straight through).
2. **Selective static override** — only if `pkgsStatic.<x>` fails to build/link.
   Build from a lighter base and inject only the needed static archive(s) via
   `.override { <lib> = pkgsStatic.<lib>; }` or by pointing a module's build at a
   `pkgsStatic.<lib>` lib dir (which ships only `.a`). See `packages/exiftool`
   (XS compression modules re-pointed at `pkgsStatic.{zlib,bzip2,xz,brotli}`).
3. **Feature reduction** — only if static fails on optional features: start from
   `pkgsStatic.<tool>` and `.override` the offending features off, keeping every
   library whose `.a` links cleanly. See `packages/ffmpeg/darwin.nix` for the
   pattern (same idea applies on Linux).
4. **Go/CGO tools:** set `CGO_ENABLED=0` to drop libc entirely.
5. **Last resort only:** if a tool genuinely cannot be statically compiled
   (e.g. a Node.js runtime), use the sibling-wrapper runtime pattern; use
   `bundle = true` (Linux only) solely when no reusable sibling runtime exists.

Verify (Linux): after build, the binary must be static and free of `/nix` refs
(see Verification below).

---

## macOS — Goal: only macOS system libs may stay dynamic

Full static is impossible on darwin (no static libSystem/libc). Goal: **every
nix-internal dependency is statically linked; only macOS system libraries may
remain dynamic** — `/usr/lib/libSystem.B.dylib`, other `/usr/lib/*`, and
`/System/Library/Frameworks/*`. **No `/nix/store` dylib may survive.**

Apply this ladder, in order — **stop at the first that works, and do not patch
if `pkgsStatic.<x>` already builds**:

1. **Fix `pkgsStatic.<x>` first.** Always try to make the stock full-static
   `pkgsStatic.<x>` build/link before anything else. If it builds cleanly, use
   it directly — no patch. If it *almost* builds, prefer a small, targeted
   upstream patch to make the static archive link on darwin over abandoning the
   static route. See `packages/krb5/darwin.nix` (disables CCAPI ccache backend +
   moves a DES const so `libkrb5.a`/`libk5crypto.a` resolve; result depends only
   on `/usr/lib/libSystem`). Sometimes only a *build-time* tool needs swapping to
   the native set while the binary itself stays fully static — see
   `packages/wget/darwin-static.nix` (darwin `pkgsStatic.perl` is broken, so
   `perlPackages` is overridden to native but wget still links every dep static).
   Also consider **feature reduction** as part of fixing the static build: start
   from `pkgsStatic.<tool>` and `.override` off only the optional features whose
   static build/link fails, keeping the rest. See `packages/ffmpeg/darwin.nix`.

2. **Only if the `pkgsStatic.<x>` problem is clearly unsolvable:** fall back to
   the **native `pkgs.<x>`** derivation (prebuilt in the upstream cache, no local
   toolchain build) and swap each of its non-system dynamic dependencies for the
   matching static archive — i.e. `pkgs.<x>.override { <dep> = pkgsStatic.<dep>; }`
   (or point the build at a `pkgsStatic.<dep>` lib dir that ships only `.a`) — so
   every nix dep links statically and only `/usr/lib`/framework libs stay dynamic.
   This meets the final goal without touching Mach-O load commands. See
   `packages/perl/darwin.nix` (native perl + `libxcrypt = libxcryptStatic`) and
   `packages/wget/darwin.nix` (dep-by-dep static swap variant).

3. **If step 2 still leaves a `/nix/store` dylib you cannot static-swap: STOP and
   confirm with the user first.** Only after explicit confirmation, use the
   `install_name_tool` route to rewrite remaining `/nix/store` Mach-O install
   names to `@loader_path`-relative paths in `postInstall` (`normalize.sh` does
   **not** touch Mach-O load commands). See `packages/perl/darwin.nix` for the
   `install_name_tool -id`/`-change` loop that repoints `libperl.dylib`.

4. **Copy the dylib into the package** (dylib-bundle) — the absolute last resort,
   ONLY when a dependency cannot be statically linked by any route above. **This
   REQUIRES explicit user confirmation before you implement it.** Do not do this
   silently. When you reach this point, STOP and ask the user (with the specific
   dependency and why static linking is impossible) before proceeding. If
   confirmed: copy the dylib next to the binary and rewrite its install name /
   the consumer's load command to `@loader_path`-relative so the payload stays
   relocatable.

### macOS Mach-O relocation reminder

`normalize.sh` handles ELF (`nuke-refs`, strip) but NOT Mach-O load commands. Any
darwin route that leaves a non-system dylib in the package must, in `postInstall`:
- `install_name_tool -id "@loader_path/<name>.dylib" <dylib>`
- `install_name_tool -change "<old /nix or abs id>" "@loader_path/..." <consumer>`
- use `otool -D` to read the current id and `otool -L`/`file` to enumerate.
See `packages/perl/darwin.nix` for the full loop.

---

## Verification (do this before declaring done)

Build the standalone output and confirm no forbidden dynamic deps remain.

- **Linux** — must be fully static, no `/nix` interpreter/refs:
  - `file ./result/bin/<tool>` → expect "statically linked".
  - `ldd ./result/bin/<tool>` → "not a dynamic executable" (or no `/nix` entries).
  - No `/nix/store` strings that are real runtime deps (normalize nuke-refs the rest).
- **macOS** — every load command must point at `/usr/lib/*`,
  `/System/Library/Frameworks/*`, or a `@loader_path`-relative in-package path;
  **zero** `/nix/store` entries:
  - `otool -L ./result/bin/<tool>` (and any `.dylib`/`.bundle`/`.so` shipped).
  - Grep the tree for surviving `/nix/store` Mach-O references.

If any `/nix/store` dynamic lib survives: go back up the ladder
(`CGO_ENABLED=0`, patch install names/rpaths, static-swap the dep, or — with
confirmation — copy the dylib).

## Guardrails

- Minimal diffs: patch only what improves portability / removes dynamic deps /
  fixes runtime paths. Don't refactor unrelated code.
- Match existing package style (`overrideAttrs`, `postInstall`, wrapper scripts).
- If your change affects architecture, build/package selection, or artifact
  format, update `CLAUDE.md` in the same change (repo rule).
- **Never** copy a `/nix` dylib on macOS without explicit user confirmation.
