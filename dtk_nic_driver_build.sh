#!/bin/bash
# Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.

# Load environment variables if file exists
if [ -f "$(dirname "$0")/dtk.env" ]; then
    source "$(dirname "$0")/dtk.env"
fi

: ${USE_NEW_ENTRYPOINT:=false}

if [ "$USE_NEW_ENTRYPOINT" = "true" ]; then
    echo "Using Go entrypoint for DTK build"
    exec "$(dirname "$0")/entrypoint" dtk-build
fi

: ${ENTRYPOINT_DEBUG:=false}
: ${DTK_OCP_NIC_SHARED_DIR:=/mnt/shared-nvidia-nic-driver-toolkit}

# Sanitize kernel version to match Kubernetes NFD label format used by network-operator for volume paths
# NFD replaces all non-alphanumeric characters (except -._) with underscore, then trims leading/trailing -._
DTK_KVER=$(uname -r | sed 's/[^-A-Za-z0-9_.]/_/g' | sed 's/^[-_.]*//;s/[-_.]*$//')
DTK_OCP_NIC_SHARED_DIR=$DTK_OCP_NIC_SHARED_DIR/$DTK_KVER
DTK_OCP_START_COMPILE_FLAG=""
DTK_OCP_DONE_COMPILE_FLAG=""
DTK_OCP_COMPILED_DRIVER_VER=""

RETRY_DELAY_SEC=3

# Modified by main entrypoint prior to execution
append_driver_build_flags=""
USE_DKMS=false

function timestamp_print () {
    date_time_stamp=$(date +'%d-%b-%y_%H:%M:%S')
    echo "[${date_time_stamp}] $@"
}

function debug_print() {
    [ ${ENTRYPOINT_DEBUG} ] && timestamp_print $@
}

# Function to execute command, capture exit status and stdout
function exec_cmd() {
    debug_print "Executing command: $@"

    output=$(eval "$@")
    exit_code=$?
    echo "$output"

    if [[ $exit_code -ne 0 ]]; then
        echo "Command \"$@\" failed with exit code: $exit_code"
        exit $exit_code
    fi
}

timestamp_print "DTK driver build script start"

if [ -z "${DTK_OCP_START_COMPILE_FLAG}" ] || [ -z "${DTK_OCP_DONE_COMPILE_FLAG}" ]; then
    timestamp_print "Compilation start/completion flags not set, aborting"
    exit 1
fi

# Req. for /install.pl script
exec_cmd "dnf install -y perl"
# Req. for build
exec_cmd "dnf install -y ethtool autoconf pciutils automake libtool python3-devel"

while [ ! -f ${DTK_OCP_START_COMPILE_FLAG} ]; do
    echo "Awaiting driver container preparations prior compilation, next query in ${RETRY_DELAY_SEC} sec"
    sleep ${RETRY_DELAY_SEC}
done

timestamp_print "Starting compilation of driver version ${DTK_OCP_COMPILED_DRIVER_VER}"

COMMON_BUILD_FLAGS="--build-only --kernel-only --without-knem --without-iser --without-isert --without-srp --with-mlnx-tools --with-ofed-scripts --copy-ifnames-udev --without-xpmem --without-xpmem-modules"

if [[ "${USE_DKMS}" = true ]]; then
    # DKMS path: omit --disable-kmp so that install.pl produces both the DKMS source
    # package (for dkms add registration) and pre-compiled kmod binary packages (which
    # place .ko files without requiring kernel headers in the main container).
    exec_cmd "${DTK_OCP_NIC_SHARED_DIR}/MLNX_OFED_SRC-${DTK_OCP_COMPILED_DRIVER_VER}/install.pl ${COMMON_BUILD_FLAGS} --without-xpmem-dkms ${append_driver_build_flags}"
else
    # Non-DKMS path: suppress DKMS source packages and kmod binary packages; produce
    # only static kernel module packages.
    exec_cmd "${DTK_OCP_NIC_SHARED_DIR}/MLNX_OFED_SRC-${DTK_OCP_COMPILED_DRIVER_VER}/install.pl ${COMMON_BUILD_FLAGS} --disable-kmp --without-dkms ${append_driver_build_flags}"
fi

exec_cmd "touch ${DTK_OCP_DONE_COMPILE_FLAG}"

exec_cmd "rm ${DTK_OCP_START_COMPILE_FLAG}"

timestamp_print "DTK driver build script end"
sleep infinity &
wait $!
