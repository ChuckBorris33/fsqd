#!/usr/bin/env python3
"""Script to inject version from pyproject.toml into static/index.html."""

import re
import os
from pathlib import Path


def inject_version():
    project_root = Path(__file__).parent.parent
    pyproject_path = project_root / "pyproject.toml"
    index_html_path = project_root / "static" / "index.html"

    with open(pyproject_path, "r") as f:
        pyproject_content = f.read()

    version_match = re.search(r'version\s*=\s*"([^"]+)"', pyproject_content)
    if not version_match:
        raise ValueError("Version not found in pyproject.toml")
    version = version_match.group(1)

    with open(index_html_path, "r") as f:
        html_content = f.read()

    version_pattern = r'<span\s+id="app-version">[^<]*</span>'
    version_replacement = f'<span id="app-version">{version}</span>'
    html_content = re.sub(version_pattern, version_replacement, html_content)

    with open(index_html_path, "w") as f:
        f.write(html_content)

    print(f"Injected version {version} into index.html")


if __name__ == "__main__":
    inject_version()
