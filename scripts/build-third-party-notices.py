#!/usr/bin/env python3
"""Build release notices for the exact Go module dependency graph."""

import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LICENSE_NAMES = ("LICENSE", "LICENSE.md", "LICENSE.txt", "COPYING")


def modules():
    output = subprocess.check_output(["go", "list", "-m", "-json", "all"], cwd=ROOT, text=True)
    decoder = json.JSONDecoder()
    position = 0
    while output[position:].strip():
        remaining = output[position:]
        whitespace = len(remaining) - len(remaining.lstrip())
        object_, consumed = decoder.raw_decode(remaining.lstrip())
        position += whitespace + consumed
        if not object_.get("Main"):
            yield object_


def license_for(module):
    directory = Path(module["Dir"])
    candidates = [directory / name for name in LICENSE_NAMES]
    candidates.extend(sorted(directory.glob("LICENSE-*")))
    candidates.extend(sorted(directory.glob("COPYING*")))
    found = [candidate for candidate in candidates if candidate.is_file()]
    if found:
        return "\n\n".join(candidate.read_text(encoding="utf-8", errors="replace") for candidate in found)
    if module["Path"] == "github.com/mattn/go-localereader":
        return (ROOT / "third_party_licenses" / "go-localereader-LICENSE").read_text()
    raise RuntimeError(f"No license file for {module['Path']} {module['Version']}")


def main():
    subprocess.run(["go", "mod", "download", "all"], cwd=ROOT, check=True)
    output = ["Third-party notices for rog\n", "\n",
              "The following Go module licenses are included for release builds.\n",
              "fd and fzf inspired traversal and matching techniques; no source from either was copied.\n"]
    for module in sorted(modules(), key=lambda item: item["Path"]):
        output += ["\n", "=" * 72, "\n", module["Path"], " ", module["Version"], "\n", "\n",
                   "\n".join(line.rstrip() for line in license_for(module).splitlines()).rstrip(), "\n"]
    (ROOT / "THIRD_PARTY_NOTICES.txt").write_text("".join(output))


if __name__ == "__main__":
    main()
