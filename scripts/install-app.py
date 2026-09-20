#!/usr/bin/env python3
"""Update one fixed application path; never execute staging or backup copies."""

import argparse
import os
from pathlib import Path
import plistlib
import shutil
import signal
import subprocess
import sys
import time


def run(command):
    subprocess.run(command, check=True)


def installed_pids(destination):
    executable = str(destination / "Contents/MacOS/hello-tsnet")
    result = subprocess.check_output(["/bin/ps", "-axww", "-o", "pid=,comm="], text=True)
    matches = []
    for line in result.splitlines():
        fields = line.strip().split(None, 1)
        if len(fields) == 2 and fields[1] == executable:
            matches.append(int(fields[0]))
    return matches


def stop_installed(destination):
    for pid in installed_pids(destination):
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
    deadline = time.monotonic() + 12
    while installed_pids(destination):
        if time.monotonic() >= deadline:
            raise RuntimeError("旧应用尚未退出，已停止更新；请在菜单栏退出应用后再试。")
        time.sleep(0.2)


def install(source, destination, backup, launch):
    with (source / "Contents/Info.plist").open("rb") as file:
        info = plistlib.load(file)
    if info.get("CFBundleIdentifier") != "org.hellotsnet.app" or info.get("CFBundleExecutable") != "hello-tsnet":
        raise RuntimeError("构建产物不是 Hello TSNet 应用。")
    run(["/usr/bin/codesign", "--verify", "--strict", str(source)])
    destination.parent.mkdir(parents=True, exist_ok=True)
    if destination.is_symlink() or (destination / "Contents").is_symlink():
        raise RuntimeError("安装位置不能是符号链接。")
    stop_installed(destination)
    existed = destination.exists()
    backup.parent.mkdir(parents=True, exist_ok=True)
    if existed:
        shutil.make_archive(str(backup.with_suffix("")), "tar", destination.parent, destination.name)
    try:
        # Keep the .app directory and executable path constant across updates.
        contents = destination / "Contents"
        if contents.exists():
            shutil.rmtree(contents)
        shutil.copytree(source / "Contents", contents)
        run(["/usr/bin/codesign", "--verify", "--strict", str(destination)])
    except Exception:
        if destination.exists():
            shutil.rmtree(destination)
        if existed:
            shutil.unpack_archive(backup, destination.parent)
        raise
    if launch:
        # One Launch Services request only. Never retry after a security block,
        # and never locate an application by bundle ID (which may pick a copy).
        run(["/usr/bin/open", "-a", str(destination)])
    print(f"已更新：{destination}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--no-open", action="store_true", help="只更新，不启动应用")
    args = parser.parse_args()
    if sys.platform != "darwin":
        raise RuntimeError("安装需要 macOS。")
    root = Path(__file__).resolve().parent.parent
    install(root / "dist/hello-tsnet.app", Path("/Applications/Hello TSNet.app"),
            root / "dist/hello-tsnet-installed.previous.tar", launch=not args.no_open)


if __name__ == "__main__":
    try:
        main()
    except Exception as error:
        print(f"更新失败：{error}", file=sys.stderr)
        sys.exit(1)
