# docker-buildx CLI plugin for macOS.
{
  docker-buildx,
}:

docker-buildx.overrideAttrs (oldAttrs: {
  postInstall = (oldAttrs.postInstall or "") + ''
    plugin="$out/libexec/docker/cli-plugins/docker-buildx"
    oldResolv=$(otool -L "$plugin" | awk '/\/nix\/store\/.*libresolv/ {print $1}')
    if [ -n "$oldResolv" ]; then
      install_name_tool -change "$oldResolv" /usr/lib/libresolv.9.dylib "$plugin"
    fi
  '';
})
