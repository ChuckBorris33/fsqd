#!/usr/bin/env python3
"""Version management tool for fsqd."""

import re
import subprocess
import sys
from pathlib import Path

PROJECT_ROOT = Path(__file__).parent.parent
PYPROJECT_PATH = PROJECT_ROOT / "pyproject.toml"
INJECT_SCRIPT = PROJECT_ROOT / "scripts" / "inject_version.py"


def get_version():
    with open(PYPROJECT_PATH, "r") as f:
        content = f.read()
    match = re.search(r'version\s*=\s*"([^"]+)"', content)
    return match.group(1) if match else None


def get_last_committed_version():
    try:
        result = subprocess.run(
            ["git", "show", "HEAD:pyproject.toml"],
            capture_output=True,
            text=True,
            check=True,
            cwd=PROJECT_ROOT,
        )
        content = result.stdout
        match = re.search(r'version\s*=\s*"([^"]+)"', content)
        return match.group(1) if match else None
    except subprocess.CalledProcessError:
        # If no commits yet or error, return None
        return None


def set_version(version: str):
    with open(PYPROJECT_PATH, "r") as f:
        content = f.read()
    content = re.sub(r'(version\s*=\s)"[^"]+"', rf'\1"{version}"', content)
    with open(PYPROJECT_PATH, "w") as f:
        f.write(content)


def run_inject():
    subprocess.run([sys.executable, str(INJECT_SCRIPT)], check=True)


def bump(part: str, from_pre_commit: bool = False):
    current = get_version()
    if not current:
        raise ValueError("No current version found in pyproject.toml")
    if from_pre_commit:
        last_committed = get_last_committed_version()
        if last_committed and current != last_committed:
            print(
                f"Version already modified from {last_committed} to {current}, skipping bump"
            )
            return
    major, minor, patch = current.split(".")
    if part == "major":
        major = str(int(major) + 1)
        minor = "0"
        patch = "0"
    elif part == "minor":
        minor = str(int(minor) + 1)
        patch = "0"
    elif part == "patch":
        patch = str(int(patch) + 1)
    else:
        print(f"Unknown part: {part}")
        sys.exit(1)
    new_version = f"{major}.{minor}.{patch}"
    set_version(new_version)
    run_inject()
    print(f"Bumped version: {current} -> {new_version}")


def show():
    print(get_version())


def main():
    if len(sys.argv) < 2:
        show()
        return

    cmd = sys.argv[1]
    from_pre_commit = len(sys.argv) > 2 and sys.argv[2] == "--pre-commit"
    if cmd == "show":
        show()
    elif cmd in ("patch", "minor", "major"):
        bump(cmd, from_pre_commit)
    else:
        print(f"Usage: {sys.argv[0]} [show|patch|minor|major] [--pre-commit]")
        sys.exit(1)


if __name__ == "__main__":
    main()
