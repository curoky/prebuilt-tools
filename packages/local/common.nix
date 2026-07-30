{
  pkgs,
  pkgsStatic,
}:
{
  # Resource bundles.
  cacert = pkgsStatic.callPackage ../cacert { };
  rime-plugins = pkgsStatic.callPackage ../rime-plugins { };
  tmux-plugins = pkgsStatic.callPackage ../tmux-plugins { };
  vim-plugins = pkgs.callPackage ../vim-plugins { };
  zsh-plugins = pkgsStatic.callPackage ../zsh-plugins { };

  # C / autotools.
  autoconf = pkgsStatic.callPackage ../autoconf { };
  automake = pkgsStatic.callPackage ../automake { };
  coreutils = pkgsStatic.coreutils.override {
    singleBinary = false;
  };
  curl = pkgsStatic.callPackage ../curl { };
  diffutils = pkgsStatic.callPackage ../diffutils { };
  eza-ls = pkgsStatic.callPackage ../eza-ls { };
  file = pkgsStatic.callPackage ../file { };
  gettext = pkgsStatic.callPackage ../gettext { };
  gnupg = pkgsStatic.gnupg.override {
    enableMinimal = true;
    guiSupport = false;
  };
  libtool = pkgsStatic.callPackage ../libtool { };
  makeself = pkgsStatic.callPackage ../makeself { };
  p7zip = pkgsStatic.callPackage ../p7zip { };
  protobuf_3_8_0 = pkgsStatic.callPackage ../protobuf/3_8_0 { };
  protobuf_3_9_2 = pkgsStatic.callPackage ../protobuf/3_9_2 { };
  vim = pkgsStatic.callPackage ../vim { };
  zsh = pkgsStatic.callPackage ../zsh { };

  # Sibling-runtime wrappers.
  git-filter-repo = pkgs.callPackage ../git-filter-repo { };
  netron = pkgs.callPackage ../netron { };
  cloc = pkgs.callPackage ../cloc { };
  parallel = pkgs.callPackage ../parallel { };

  # glibc-dynamic AOT exception.
  music-decrypto = pkgs.callPackage ../music-decrypto { };
}
