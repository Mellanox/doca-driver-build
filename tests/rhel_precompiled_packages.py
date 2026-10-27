#!/usr/bin/env python3
# Copyright 2026 NVIDIA CORPORATION & AFFILIATES
# SPDX-License-Identifier: Apache-2.0

"""Exercise the actual precompiled package-install RUN with public UBI repos.

Run from any directory with Python 3 and Docker Buildx available. This tests
package availability without needing DTK credentials or building the driver;
it does not qualify a DKMS module rebuild or hardware operation.
"""

from pathlib import Path
import subprocess
import tempfile


def package_install_run(dockerfile):
    stage = dockerfile.split("FROM $D_FINAL_BASE_IMAGE AS precompiled\n", 1)[1]
    # Dockerfile comments may occur inside a continued RUN instruction.
    lines = [line for line in stage.splitlines() if not line.lstrip().startswith("#")]
    instructions = "\n".join(lines).replace("\\\n", " ").splitlines()
    matches = [
        line for line in instructions
        if line.startswith("RUN ") and "epel-release" in line
    ]
    if len(matches) != 1:
        raise ValueError("Expected one precompiled EPEL/package-install RUN")
    return matches[0]


def main():
    source = Path(__file__).resolve().parents[1] / "RHEL_Dockerfile"
    install = package_install_run(source.read_text())
    with tempfile.TemporaryDirectory(prefix="rhel-package-test-") as directory:
        dockerfile = Path(directory) / "Dockerfile"
        for enabled in ("false", "true"):
            assertion = (
                "rpm -q dkms gcc make elfutils-libelf-devel"
                if enabled == "true" else "! rpm -q dkms"
            )
            dockerfile.write_text(
                "FROM registry.access.redhat.com/ubi9/ubi:latest\n"
                "ARG D_OS=rhel9.8\n"
                f"ARG D_ENABLE_DKMS={enabled}\n"
                f"{install}\n"
                f"RUN {assertion}\n"
            )
            print(f"Testing UBI9 precompiled packages: D_ENABLE_DKMS={enabled}", flush=True)
            subprocess.run(
                ["docker", "buildx", "build", "--platform=linux/amd64",
                 "--progress=plain", "--no-cache", directory], check=True,
                timeout=900,
            )


if __name__ == "__main__":
    main()
