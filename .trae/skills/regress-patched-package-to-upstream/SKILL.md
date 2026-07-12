---
name: "regress-patched-package-to-upstream"
description: "Check whether a locally patched package under packages/ can drop its patch and go back to the plain upstream nixpkgs build (because nixpkgs fixed the issue), then perform the regression. Invoke when reviewing/updating packages/ entries, bumping nixpkgs, or auditing whether local patches are still needed."
---

# Regress a Patched Package Back to Upstream

Local patched derivations in `packages/` exist **only** to work around a
`nixpkgs` (usually `pkgsStatic.<x>`) build/link failure or portability problem.
Once upstream nixpkgs fixes that problem, the patch is dead weight: it should be
removed and the package regressed to the plain upstream build. Use this skill to
verify a patch is obsolete and to perform the regression cleanly.

Read `CLAUDE.md` first — the patch policy and package-selection model live there.
This skill is the inverse of `patch-nixpkgs-standalone`: that one adds a patch
when the stock static build fails; this one removes it when the stock build works
again.

## Principle

**Prefer the upstream package whenever it builds and meets the portability goal.**
A local package is a liability (maintenance, drift from upstream, larger diff).
If `nixpkgs.pkgsStatic.<x>` (or the appropriate stock build) now compiles, links,
and passes the portability check, delete the local patch and use upstream via the
manifest.

## First: is this package even a regression candidate?

The deciding question is **what would happen to this override if upstream had no
bugs at all.**

- If a bug-free upstream would make the override *empty* — you'd have nothing
  left to write — the override is a pure **workaround**, and it is a regression
  candidate. Its entire reason to exist is an upstream defect; remove the defect
  and the override has no content.
- If a bug-free upstream would *still* leave the override with work to do — it
  would keep doing that work regardless — the override encodes a **deliberate
  packaging decision**, not a workaround. It is never a regression candidate,
  because no upstream fix can ever cover it.

So classify by the override's *intent*, not its mechanics (the same `overrideAttrs`
call can be either):

- **Workaround (candidate).** Every line traces to "the stock build fails / links
  wrong / isn't portable." It disables a broken check, toggles off a feature that
  won't build, swaps an input to dodge a link failure, adjusts flags to satisfy
  the static toolchain. The produced program is byte-for-byte what upstream
  *intends* to ship; the override only removes the obstacle to building it. When
  nixpkgs fixes the obstacle, the override becomes a no-op — that's the signal to
  regress. (e.g. blanking a failing `installCheckPhase`; `--disable-gpg-test` +
  `doCheck = false`; dropping a flag that bakes an absolute store path so the
  binary is relocatable.)

- **Packaging decision (not a candidate).** The override changes *what the package
  is or how it's consumed* in a way upstream neither offers nor should: it renames
  the real binary behind a wrapper, redirects runtime lookups to sibling packages
  or `$PATH`, bundles extra runtime deps into `$out`, or ships vendored
  scripts/config. The value is this repackaging itself. A bug-free upstream still
  wouldn't do any of it, so there is nothing to "fix back to." (e.g. renaming a
  binary to `_name` and dropping in a wrapper script; wrapping a tool to run
  against a sibling `perl` and vendoring its Perl deps.)

The litmus test: mentally assume upstream is perfect, then ask "is the override
now empty?" Empty ⇒ candidate. Still has a job ⇒ leave it alone. When you can't
tell (the intent isn't clear from the code or its comment), ask before touching
it.

A single override can contain both — a workaround part *and* a packaging part.
Only the workaround part is regressable; keep the packaging part. See step 4's
"partial regression" note.

## When to run this

- Auditing/reviewing entries under `packages/`.
- After bumping a `nixpkgs` input (`unstable`/`26.05`/... in `flake.nix`).
- When a package's `default.nix`/`darwin.nix` comment says it patches around a
  specific upstream bug — check if that bug is gone.

## Procedure

1. **Identify why the patch exists.** Read the local
   `packages/<pkg>/*.nix` and its comments. First classify it using the
   "regression candidate?" section above — if it is intentional repackaging (a
   wrapper, renamed binaries, or bundled sibling deps), **stop**; it is not
   regressable. Otherwise the comment almost always names the exact failure
   (e.g. "darwin `pkgsStatic.perl` fails at `mktables`", "`liboapv` ships only a
   `.dylib`"). That failure is your regression test.

2. **Test the stock upstream build.** Build the plain upstream derivation the
   patch was replacing, for the target platform(s):
   - Full static route: `nix build nixpkgs#pkgsStatic.<x>` (matching the pinned
     input the manifest would use).
   - If that's what the patch worked around and it now **builds and links**, the
     patch is a candidate for removal.
   - Reproduce on **every** platform the package targets (Linux + darwin). A fix
     on one platform does not imply the other.

3. **Verify the upstream build still meets the portability goal** (same checks as
   `patch-nixpkgs-standalone`):
   - **Linux:** `file`/`ldd` → statically linked, no `/nix` runtime refs.
   - **macOS:** `otool -L` on the binary and every shipped `.dylib`/`.bundle`/`.so`
     → only `/usr/lib/*`, `/System/Library/Frameworks/*`, or `@loader_path`
     entries; **zero** `/nix/store`.
   - If upstream builds but still fails portability, the patch is **not** yet
     obsolete — keep it. Only regress when upstream is both buildable AND portable.

4. **Perform the regression** once verified:
   - If the whole package can use upstream directly, **delete** `packages/<pkg>/`
     and remove its `callPackage ./<pkg>` line from
     [packages/local.nix](file:///workspace/standalone-binaries/packages/local.nix)
     (`common`/`linux`/`darwin` set), then add/adjust the entry in
     [manifests/default.nix](file:///workspace/standalone-binaries/manifests/default.nix)
     (`isStatic = true`, correct `version`/`platforms`/`output`/`alias`).
   - If only **one platform** is fixed, drop just that platform's local file
     (e.g. remove `darwin.nix`, keep `default.nix`) and switch that platform to
     the manifest/upstream build; keep the still-needed platform patched.
   - If the patch only *partially* regressed (e.g. a feature-reduction override no
     longer needs one disabled feature), shrink the override to the minimum still
     required rather than removing the whole package.

5. **Clean up orphans your change created:** unused patch files, wrapper scripts,
   vendored configs under `packages/<pkg>/`, and now-dead `callPackage` wiring.
   Do not remove unrelated pre-existing code.

6. **Rebuild the final standalone output** for the affected package(s) and re-run
   the verification checks to confirm the regressed build is still portable.

7. **Update `CLAUDE.md`** in the same change if the regression alters the package
   selection model (e.g. a package moves from a `packages/` local override to a
   manifest entry, or a documented per-platform strategy no longer applies).

## Guardrails

- Only bug/portability patches are regressable. Never delete an intentional
  repackaging override (wrapper scripts, renamed binaries, sibling/bundled deps)
  — see "is this package even a regression candidate?". Upstream build fixes do
  not make repackaging obsolete.
- Do not regress on assumption — always build and verify upstream on **all**
  target platforms first.
- Only regress when upstream is both **buildable** and **portable**. Buildable
  but non-portable is not a reason to drop the patch.
- Keep the diff surgical: remove only what the obsolete patch owned.
- If unsure whether a platform is truly fixed (e.g. can't test darwin locally),
  say so and ask before deleting that platform's patch.
