{
  termcap,
}:
{
  python,
  setupLocal,
}:
python.overrideAttrs (oldAttrs: {
  configureFlags = oldAttrs.configureFlags ++ [
    "LDFLAGS=-L${termcap}/lib"
  ];
  stripIdlelib = true;
  stripTests = true;
  stripTkinter = true;
  postPatch = oldAttrs.postPatch + ''
    cp ${setupLocal} Modules/Setup.local
  '';
})
