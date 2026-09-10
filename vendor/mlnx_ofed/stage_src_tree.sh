#!/bin/bash
# Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#
# Reshape a DOCA 3.6+ DOCA_BASE_SRC source directory into the MLNX_OFED_SRC tree layout
# that install.pl expects, and stage the vendored installer scripts into it.
#
# DOCA 3.6 ships a flat directory of source packages and no installer scripts. install.pl
# resolves every path relative to its own location (see ../README.md), so the source
# packages have to live in SRPMS/ (RPM) or SOURCES/ (Debian) next to a copy of the script.
#
# Usage: stage_src_tree.sh <doca-base-src-dir> <target-dir> <rpm|deb>

set -euo pipefail

VENDOR_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ "$#" -ne 3 ]; then
    echo "usage: $(basename "$0") <doca-base-src-dir> <target-dir> <rpm|deb>" >&2
    exit 1
fi

SRC_DIR="$1"
TARGET_DIR="$2"
FLAVOR="$3"

case "${FLAVOR}" in
    rpm)
        # install.pl reads $CWD/SRPMS/ and writes $CWD/RPMS/<dist>/<arch>/
        SOURCES_SUBDIR="SRPMS"
        SOURCES_GLOB="*.src.rpm"
        # Sanity anchor: without the kernel sources there is nothing to build
        KERNEL_PKG_GLOB="mlnx-ofa_kernel-*.src.rpm"
        ;;
    deb)
        # The Debian install.pl hardcodes the literal string "SOURCES" when deriving
        # package versions, so this directory name is not configurable
        SOURCES_SUBDIR="SOURCES"
        SOURCES_GLOB="*.orig.tar.*"
        KERNEL_PKG_GLOB="mlnx-ofed-kernel_*.orig.tar.*"
        ;;
    *)
        echo "error: flavor must be 'rpm' or 'deb', got '${FLAVOR}'" >&2
        exit 1
        ;;
esac

if [ ! -d "${SRC_DIR}" ]; then
    echo "error: source directory '${SRC_DIR}' does not exist" >&2
    exit 1
fi

echo "Staging ${FLAVOR} source tree: ${SRC_DIR} -> ${TARGET_DIR}"

mkdir -p "${TARGET_DIR}/${SOURCES_SUBDIR}"

shopt -s nullglob
sources=("${SRC_DIR}"/${SOURCES_GLOB})
kernel_pkg=("${SRC_DIR}"/${KERNEL_PKG_GLOB})
shopt -u nullglob

if [ "${#sources[@]}" -eq 0 ]; then
    echo "error: no ${SOURCES_GLOB} found in '${SRC_DIR}'" >&2
    exit 1
fi

if [ "${#kernel_pkg[@]}" -eq 0 ]; then
    echo "error: no ${KERNEL_PKG_GLOB} found in '${SRC_DIR}'" >&2
    exit 1
fi

mv "${sources[@]}" "${TARGET_DIR}/${SOURCES_SUBDIR}/"
echo "Moved ${#sources[@]} source package(s) into ${SOURCES_SUBDIR}/"

install -m 0755 "${VENDOR_DIR}/${FLAVOR}/install.pl" "${TARGET_DIR}/install.pl"
install -m 0755 "${VENDOR_DIR}/${FLAVOR}/uninstall.sh" "${TARGET_DIR}/uninstall.sh"
install -m 0755 "${VENDOR_DIR}/common.pl" "${TARGET_DIR}/common.pl"
install -m 0644 "${VENDOR_DIR}/LICENSE" "${TARGET_DIR}/LICENSE"

# Fail here rather than midway through rpmbuild/dpkg-buildpackage
for f in install.pl common.pl; do
    if [ ! -x "${TARGET_DIR}/${f}" ]; then
        echo "error: ${TARGET_DIR}/${f} is missing or not executable" >&2
        exit 1
    fi
done

# Only remove the extracted archive directory once its contents have been claimed
if [ "$(readlink -f "${SRC_DIR}")" != "$(readlink -f "${TARGET_DIR}")" ]; then
    rmdir "${SRC_DIR}" 2>/dev/null || echo "note: '${SRC_DIR}' not empty, leaving in place"
fi

echo "Staged source tree at ${TARGET_DIR}:"
ls -la "${TARGET_DIR}"
