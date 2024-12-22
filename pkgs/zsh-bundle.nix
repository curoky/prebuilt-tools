{ lib, stdenv, fetchFromGitHub, oh-my-zsh,
  zsh-syntax-highlighting,
  zsh-autosuggestions,
  zsh-completions,
  zsh-fast-syntax-highlighting,
  starship,
  atuin,
  fzf,
}:

stdenv.mkDerivation rec {
  version = "1.0.0";
  pname = "zsh-bundle";

  srcs = [
    (fetchFromGitHub {
      owner = "conda-incubator";
      repo = "conda-zsh-completion";
      rev = "v0.11";
      sha256 = "sha256-OKq4yEBBMcS7vaaYMgVPlgHh7KQt6Ap+3kc2hOJ7XHk=";
      name = "conda-zsh-completion";
    })
  ];

  sourceRoot = ".";
  strictDeps = true;
  buildInputs = [
    #starship
    #atuin
    #fzf
  ];

  installPhase = ''
    cp -r ${oh-my-zsh.out} $out/
    chmod +w $out/share/oh-my-zsh/custom/plugins/
    cp -r ${zsh-autosuggestions.src}/ $out/share/oh-my-zsh/custom/plugins/zsh-autosuggestions
    cp -r ${zsh-syntax-highlighting.src}/ $out/share/oh-my-zsh/custom/plugins/zsh-syntax-highlighting
    cp -r ${zsh-completions.src}/ $out/share/oh-my-zsh/custom/plugins/zsh-completions
    cp -r ${zsh-fast-syntax-highlighting.src}/ $out/share/oh-my-zsh/custom/plugins/zsh-fast-syntax-highlighting
    cp -r conda-zsh-completion $out/share/oh-my-zsh/custom/plugins/

    chmod +w $out/share
    mkdir -p $out/share/zsh/site-functions
    cp $out/share/oh-my-zsh/custom/plugins/conda-zsh-completion/_conda $out/share/zsh/site-functions

    # mkdir -p starship/
    # ${starship}/bin/starship init zsh >starship/starship.plugin.zsh
    # ${starship}/bin/starship completions zsh >starship/_starship
    # mkdir -p atuin/
    # export ATUIN_CONFIG_DIR=atuin/
    # ${atuin}/bin/ zsh --disable-up-arrow >atuin/atuin.plugin.zsh
    # ${atuin}/bin/atuin gen-completions --shell zsh >atuin/_atuin
  '';

  meta = with lib; {
    description = "zsh bundle";
  };
}
