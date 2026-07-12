# macOS ffmpeg (headless), `pkgsStatic` "partial static" route — CLAUDE.md
# darwin portability strategy 2: link every nix dependency statically (.a) and
# let only the macOS system libraries (/usr/lib/*, /System/Library/Frameworks/*)
# stay dynamic. A fully-static ffmpeg is impossible on darwin (no static
# libSystem) and even the full `pkgsStatic.ffmpeg` does not build here, so this
# takes `pkgsStatic.ffmpeg-headless` and disables the feature libraries that
# fail to *build* or *link* statically on aarch64-darwin, keeping the codecs
# whose static archives do link cleanly.
#
# Why these features are disabled (root causes, all darwin `pkgsStatic`
# specific):
#
#   * meson arm64 cross-file bug — on aarch64-darwin `pkgsStatic` emits a meson
#     cross-file with CPU family "arm64" instead of "aarch64", so the *static*
#     builds of these meson libraries fail: libopus (arm64 intrinsics error),
#     dav1d, speex, and the fontconfig/harfbuzz/freetype/libass chain. Hence
#     withOpus / withDav1d / withSpeex / withFontconfig / withHarfbuzz /
#     withFreetype / withAss = false. libbluray / libopenmpt / vmaf pull in the
#     same broken chain (or an autogen step that gets SIGKILLed), so they are off
#     too.
#
#   * openmp -> llvm-static libatomic — zimg, vid-stab and OpenCL (ocl-icd) all
#     drag in openmp, which forces building llvm-static; that fails on darwin at
#     the CheckAtomic (libatomic) stage. Disabling them (withZimg / withVidStab /
#     withOpencl = false) also keeps `nix eval` clean: openmp is the only path
#     that would mark python3 broken, so the flake evaluates without needing any
#     config.problems handler.
#
#   * liboapv only ships a .dylib — even under `pkgsStatic`, openapv produces
#     liboapv.2.dylib (no usable static archive), so an ffmpeg linked against it
#     would keep a /nix/store/*.dylib load command, violating the portability
#     rule. Disabled via withOpenapv = false.
#
#   * network / TLS libs (gnutls, libssh, srt, rist) fail ffmpeg's *static*
#     configure link test in this cross setup (missing transitive deps / C++
#     runtime that a non-static pkg-config probe does not surface). They are an
#     acceptable degradation (no built-in TLS for https, no sftp, no SRT/RIST
#     transport) and are disabled.
#
# x265 (H.265/HEVC encoder) is KEPT, but needs two fixes to link statically:
#
#   1. nixpkgs' x265 `postInstall` unconditionally runs `rm -f $out/lib/*.a`.
#      That is correct for a normal (shared) build, but under `pkgsStatic`
#      ENABLE_SHARED=false means the *only* library produced is `libx265.a`, and
#      that rm deletes it — leaving the x265 output with a CLI binary but no
#      library, so ffmpeg's configure reports "x265 not found using pkg-config"
#      (ld: library not found for -lx265). We drop that postInstall step so the
#      static archive survives.
#
#   2. x265's multi-bit-depth build links the 10/12-bit encoders as separate
#      libx265-10.a / libx265-12.a merged only into the *shared* lib; the static
#      `libx265.a` then references `x265_10bit::` / `x265_12bit::` symbols that
#      are not present in any installed static archive (undefined symbols at
#      ffmpeg's configure link test). Building single bit-depth
#      (multibitdepthSupport = false) removes that dependency. Trade-off: 8-bit
#      HEVC encoding only (no 10/12-bit HEVC encode); 8-bit encode + HEVC decode
#      remain.
#
# Kept codecs of note: H.264 (libx264), HEVC/H.265 8-bit encode (libx265), AV1
# (libaom + libsvtav1), VP8/VP9 decode, plus vorbis/theora/mp3(lame)/webp/
# openjpeg/soxr/xml2/zvbi and the VideoToolbox hardware encoders. The resulting
# ffmpeg binary depends only on /usr/lib/* and /System/Library/Frameworks/*.
{
  stdenv,
  ffmpeg-headless,
}:

let
  ffmpeg_static =
    (ffmpeg-headless.override (prev: {
      x265 = (prev.x265.override { multibitdepthSupport = false; }).overrideAttrs (_: {
        postInstall = "";
      });
      withOpenapv = false;
      withOpus = false;
      withDav1d = false;
      withFontconfig = false;
      withHarfbuzz = false;
      withAss = false;
      withFreetype = false;
      withSpeex = false;
      withBluray = false;
      withOpenmpt = false;
      withVmaf = false;
      withZimg = false;
      withVidStab = false;
      withOpencl = false;
      withGnutls = false;
      withSsh = false;
      withSrt = false;
      withRist = false;
    })).bin;
in

stdenv.mkDerivation {
  pname = "ffmpeg";
  version = ffmpeg-headless.version;

  dontUnpack = true;

  installPhase = ''
    mkdir -p $out
    cp -r ${ffmpeg_static}/bin $out/bin
  '';
}
