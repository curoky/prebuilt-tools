#!/usr/bin/env python3
# Copyright (c) 2024-2025 curoky(cccuroky@gmail.com).
#
# This file is part of prebuilt-tools.
# See https://github.com/curoky/prebuilt-tools for further info.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import logging
import os
import shutil
import stat
import subprocess
from pathlib import Path


def is_static_binary(file_path):
    try:
        result = subprocess.run(
            ["ldd", file_path],
            capture_output=True,
            text=True,
            check=True,
        )
        output = result.stdout

        if "statically linked" in output:
            return True
        else:
            logging.error(
                "%s is not statically linked, %s", file_path, output + result.stderr
            )
            return False
    except Exception as e:
        logging.error("Failed to check if %s is statically linked: %s", file_path, e)
        return False


def readlink(path: Path) -> Path:
    while True:
        if path.is_symlink():
            path = path.readlink()
        else:
            return path


def pack(output_path: Path, nix_paths: list[Path]):
    package_paths: set[Path] = set()

    for path in nix_paths:
        if path.exists():
            for root, dirs, files in os.walk(path, topdown=True, followlinks=False):
                for file in map(lambda x: Path(root) / Path(x), dirs):
                    # print(file)
                    if file.is_symlink():
                        link = readlink(file)
                        # print("link", link)
                        if link.as_posix().startswith("/nix/store/"):
                            # parts = path.parts
                            # if len(parts)
                            # print("start ", link.parts[:4])
                            package_paths.add(Path(*link.parts[:4]))

                for file in map(lambda x: Path(root) / Path(x), files):
                    # print(file)
                    if file.is_symlink():
                        link = readlink(file)
                        # print("link", link)
                        if link.as_posix().startswith("/nix/store/"):
                            # parts = path.parts
                            # if len(parts)
                            # print("start ", link.parts[:4])
                            package_paths.add(Path(*link.parts[:4]))

            # for subpath in path.iterdir():
            #     if subpath.is_dir():
            #         if subpath.is_symlink():
            #             link = readlink(subpath)
            #             if link.as_posix().startswith("/nix/store/"):
            #                 package_paths.add(link.parent)
            #         for subdir in subpath.iterdir():
            #             if subdir.is_symlink():
            #                 link = readlink(subdir)
            #                 if link.as_posix().startswith("/nix/store/"):
            #                     package_paths.add(link.parent.parent)

    print(package_paths)
    # exit(1)
    for path in package_paths:
        for root, _, files in os.walk(path, topdown=True, followlinks=False):
            root = Path(root)
            for file in files:
                abs_path = root / file

                if abs_path.is_symlink() and abs_path.is_dir():
                    continue
                if "/bin/" in abs_path.as_posix():
                    if not (abs_path.stat().st_mode & stat.S_IXUSR):
                        logging.error("file %s is not executable", abs_path)
                        # exit(-1)
                    # if not is_static_binary(abs_path.as_posix()):
                    #     logging.error("file %s is not static binary", abs_path)
                    #     exit(-1)
                target_path = output_path / (root.relative_to(path) / file)
                os.makedirs(target_path.parent, exist_ok=True)
                if target_path.exists():
                    logging.warning("file %s exists", target_path)
                    continue
                logging.info("cp %s to %s", abs_path, target_path)
                shutil.copy(abs_path, target_path, follow_symlinks=False)


if __name__ == "__main__":
    logging.basicConfig(format="%(levelname)s:%(message)s", level=logging.INFO)
    shutil.rmtree("tmp/output/", ignore_errors=True)
    os.makedirs("tmp/output/")
    pack(
        output_path=Path("tmp/output"),
        nix_paths=[Path.home() / "nix/profiles/prebuilt"],
    )
