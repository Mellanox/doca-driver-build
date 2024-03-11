#!/bin/bash
# Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.

: ${ENTRYPOINT_DEBUG:=false}
: ${DTK_OCP_NIC_SHARED_DIR:=/mnt/shared-nvidia-nic-driver-toolkit}

DTK_OCP_START_COMPILE_FLAG=""
DTK_OCP_DONE_COMPILE_FLAG=""
DTK_OCP_COMPILED_DRIVER_VER=""

RETRY_DELAY_SEC=3

# Modified by main entrypoint prior to execution
append_driver_build_flags=""

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

exec_cmd "${DTK_OCP_NIC_SHARED_DIR}/MLNX_OFED_SRC-${DTK_OCP_COMPILED_DRIVER_VER}/install.pl --build-only --kernel-only --without-knem --without-iser --without-isert --without-srp  --with-mlnx-tools --with-ofed-scripts --copy-ifnames-udev ${append_driver_build_flags}"

exec_cmd "touch ${DTK_OCP_DONE_COMPILE_FLAG}"

exec_cmd "rm ${DTK_OCP_START_COMPILE_FLAG}"

timestamp_print "DTK driver build script end"
sleep infinity & wait
