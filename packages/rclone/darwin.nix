{
  nukeReferences,
  rclone,
}:

rclone.overrideAttrs (oldAttrs: {
  nativeBuildInputs = (oldAttrs.nativeBuildInputs or [ ]) ++ [ nukeReferences ];

  postInstall = (oldAttrs.postInstall or "") + ''
    oldResolv=$(otool -L "$out/bin/rclone" | awk '/\/nix\/store\/.*libresolv/ { print $1 }')
    if [ -n "$oldResolv" ]; then
      install_name_tool \
        -change "$oldResolv" /usr/lib/libresolv.9.dylib \
        "$out/bin/rclone"
    fi
    nuke-refs "$out/bin/rclone"
  '';
})
