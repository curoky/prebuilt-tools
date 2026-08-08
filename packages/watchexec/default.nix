{
  watchexec,
}:

watchexec.overrideAttrs (oldAttrs: {
  cargoBuildFlags = (oldAttrs.cargoBuildFlags or [ ]) ++ [
    "--package=watchexec-cli"
  ];
})
