"""Bounded Ubuntu archive fallback for GitHub-hosted CI; keep apt signatures."""
from pathlib import Path
import os
import re
import subprocess

PRIMARY = "https://archive.ubuntu.com/ubuntu"
MIRRORS = ("https://mirrors.edge.kernel.org/ubuntu", "https://mirror.math.princeton.edu/pub/ubuntu")
MIRROR_FILE = Path("/etc/apt/apt-mirrors.txt")
SOURCES = (Path("/etc/apt/sources.list"), Path("/etc/apt/sources.list.d/ubuntu.sources"))
CANONICAL = re.compile(r"https?://(?:[a-z0-9-]+\.)?(?:archive|security)\.ubuntu\.com/ubuntu/?", re.I)

def release_valid(content, codename):
    return (content.startswith(b"-----BEGIN PGP SIGNED MESSAGE-----") and
            b"\nOrigin: Ubuntu\n" in content and
            ("\nCodename: " + codename + "\n").encode() in content)

def probe(base, codename):
    url = base + "/dists/" + codename + "/InRelease"
    try:
        result = subprocess.run(["curl", "--fail", "--silent", "--show-error",
            "--location", "--connect-timeout", "5", "--max-time", "15",
            "--proto", "=https", "--proto-redir", "=https", url],
            capture_output=True, timeout=20, check=False)
        return result.returncode == 0 and len(result.stdout) <= 2*1024*1024 and release_valid(result.stdout, codename)
    except (OSError, subprocess.TimeoutExpired):
        return False

def plan_sources(files, mirror):
    if mirror not in MIRRORS:
        raise ValueError("unapproved Ubuntu archive mirror")
    changes = {}
    for path in SOURCES:
        if path not in files:
            continue
        value = CANONICAL.sub(mirror, files[path])
        if value != files[path]:
            changes[path] = value
        if "mirror+file:" + MIRROR_FILE.as_posix() in files[path]:
            changes[MIRROR_FILE] = mirror + "\n"
    if not changes:
        raise RuntimeError("No recognized Ubuntu package source; leave apt configuration unchanged")
    return changes

def prepare(files, codename, check=probe):
    if not re.fullmatch(r"[a-z][a-z0-9-]{0,30}", codename):
        raise ValueError("invalid Ubuntu release codename")
    if check(PRIMARY, codename):
        return {}
    for mirror in MIRRORS:
        if check(mirror, codename):
            return plan_sources(files, mirror)
    raise RuntimeError("No reachable Ubuntu HTTPS archive; do not bypass dependency installation")

def main():
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise SystemExit("This helper only changes GitHub Actions runner apt configuration")
    release = dict(line.split("=", 1) for line in Path("/etc/os-release").read_text().splitlines() if "=" in line)
    if release.get("ID", "").strip('"') != "ubuntu":
        raise SystemExit("Expected an Ubuntu runner")
    codename = release.get("VERSION_CODENAME", "").strip('"')
    files = {path: path.read_text() for path in SOURCES if path.exists()}
    changes = prepare(files, codename)
    if not changes:
        print("Canonical Ubuntu archive is reachable; package sources unchanged")
        return
    for path, value in changes.items():
        subprocess.run(["sudo", "-n", "tee", str(path)], input=value.encode(),
            stdout=subprocess.DEVNULL, check=True, timeout=10)
    print("Selected a reachable Ubuntu HTTPS archive; Signed-By and apt verification remain unchanged")

if __name__ == "__main__":
    main()
