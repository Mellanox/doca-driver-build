#!/bin/bash
# Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
#
# This software is available to you under a choice of one of two
# licenses.  You may choose to be licensed under the terms of the GNU
# General Public License (GPL) Version 2, available from the file
# COPYING in the main directory of this source tree, or the
# OpenIB.org BSD license below:
#
#     Redistribution and use in source and binary forms, with or
#     without modification, are permitted provided that the following
#     conditions are met:
#
#      - Redistributions of source code must retain the above
#        copyright notice, this list of conditions and the following
#        disclaimer.
#
#      - Redistributions in binary form must reproduce the above
#        copyright notice, this list of conditions and the following
#        disclaimer in the documentation and/or other materials
#        provided with the distribution.
#
# THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
# EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
# MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
# NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS
# BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN
# ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
# CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

: ${UNLOAD_STORAGE_MODULES:=false}
: ${CREATE_IFNAMES_UDEV:=false}
: ${ENABLE_NFSRDMA:=false}
: ${RESTORE_DRIVER_ON_POD_TERMINATION:=true}

: ${ENTRYPOINT_DEBUG:=false}
: ${DEBUG_LOG_FILE:=/tmp/entrypoint_debug_cmds.log}
: ${DEBUG_SLEEP_SEC_ON_EXIT:=300}

: ${NVIDIA_NIC_DRIVER_VER:=""}
: ${NVIDIA_NIC_DRIVER_PATH:=""}
: ${NVIDIA_NIC_CONTAINER_VER:=""}

: ${DTK_OCP_DRIVER_BUILD:=false}
: ${DTK_OCP_NIC_SHARED_DIR:=/mnt/shared-nvidia-nic-driver-toolkit}

: ${NVIDIA_NIC_DRIVERS_INVENTORY_PATH:=""}

function timestamp_print () {
    date_time_stamp=$(date +'%d-%b-%y_%H:%M:%S')
    msg="[${date_time_stamp}] $@"
    echo "${msg}" | tee -a ${DEBUG_LOG_FILE}
}

function debug_print() {
    if ${ENTRYPOINT_DEBUG}; then
        timestamp_print $@
    fi
}

function exit_entryp() {
    debug_print "Function: ${FUNCNAME[0]}"

    if ${ENTRYPOINT_DEBUG}; then
        debug_print "Entrypoint exit request caught - exit code: $1, attempting to sleep ${DEBUG_SLEEP_SEC_ON_EXIT} sec."
        sleep ${DEBUG_SLEEP_SEC_ON_EXIT}
    fi

    exit $1
}

# Commands success for destroy methods is irrelevant
function unmount_rootfs() {
    debug_print "Function: ${FUNCNAME[0]}"

    if [ `findmnt -r -o TARGET | grep ${MLX_DRIVERS_MOUNT} -c` -gt 1 ] ; then
        debug_print "Unmounting ${MLX_DRIVERS_MOUNT}"
        umount -l -R ${MLX_DRIVERS_MOUNT} > /dev/null 2>&1
        rm -rf ${MLX_DRIVERS_MOUNT}${SHARED_KERNEL_HEADERS_DIR}
    fi
}

function delete_udev_rules() {
    debug_print "Function: ${FUNCNAME[0]}"

    if [ -f ${MLX_UDEV_RULES_FILE} ]; then
        timestamp_print "Deleting udev rules"
        rm ${MLX_UDEV_RULES_FILE}

    else
        debug_print "udev rules file was not previously created, skipping"
    fi
}

function set_driver_readiness() {
    debug_print "Function: ${FUNCNAME[0]}"

    if [ "$1" == "1" ]; then
        timestamp_print "Setting driver ready state"
        exec_cmd "touch ${DRIVER_READY_FILE}"
    else
        timestamp_print "Unsetting driver ready state"
        exec_cmd "rm -f ${DRIVER_READY_FILE}"
    fi
}

function termination_handler() {
    debug_print "Function: ${FUNCNAME[0]}"

    timestamp_print "Terminating container"

    set_driver_readiness 0

    if [[ "${CREATE_IFNAMES_UDEV}" = true ]]; then
        delete_udev_rules
    fi

    if [[ "${RESTORE_DRIVER_ON_POD_TERMINATION}" != false  && "${new_driver_loaded}" == true ]]; then
        if [ -e /usr/sbin/mlnxofedctl ]; then
            timestamp_print "Restoring Mellanox OFED Driver from host..."
            /usr/sbin/mlnxofedctl --alt-mods force-restart

            print_loaded_drv_ver_str

            restore_sriov_config
        fi
    else
        timestamp_print "Keeping currently loaded Mellanox OFED Driver..."
    fi

    unmount_rootfs

    if ! ${reuse_driver_inventory} || ${driver_build_incomplete}; then
        debug_print "Removing driver packages temporary directory ${driver_inventory_path}"
        exec_cmd "rm -rf ${driver_inventory_path}"
    fi
}

function terminate_event() {
    debug_print "Function: ${FUNCNAME[0]}"

    timestamp_print "Terminate event caught"

    termination_handler

    trap - EXIT SIGTERM SIGINT

    exit_entryp 1
}

# Function to execute command, capture exit status and stdout
function exec_cmd() {
    debug_print "Executing command: $@"

    output=$(eval "$@")
    exit_code=$?
    echo "$output"

    if [[ $exit_code -ne 0 ]]; then
        timestamp_print "Command \"$@\" failed with exit code: $exit_code"
        exit_entryp $exit_code
    fi
}

function mount_rootfs() {
    debug_print "Function: ${FUNCNAME[0]}"

    timestamp_print "Mounting Mellanox OFED driver container shared kernel headers"
    exec_cmd mount --make-runbindable /sys
    exec_cmd mount --make-private /sys

    (mount -l | grep mellanox | grep -v tmpfs -q) && debug_print "Mount ${MLX_DRIVERS_MOUNT} exists" && return

    exec_cmd mkdir -p ${MLX_DRIVERS_MOUNT}${SHARED_KERNEL_HEADERS_DIR}
    exec_cmd mount --rbind ${SHARED_KERNEL_HEADERS_DIR} ${MLX_DRIVERS_MOUNT}${SHARED_KERNEL_HEADERS_DIR}
}

function ubuntu_install_prerequisites() {
    debug_print "Function: ${FUNCNAME[0]}"

    if [[ ${FULL_KVER} =~ "realtime" ]]; then
        debug_print "RT kernel identified"

        exec_cmd "cp -r /host/etc/apt/* /etc/apt/"
    fi

    exec_cmd "apt-get update"
    exec_cmd "apt-get -yq install pkg-config linux-headers-${FULL_KVER}"
}

function redhat_fetch_major_ver() {
    debug_print "Function: ${FUNCNAME[0]}"

    if ${IS_OS_UBUNTU}; then
        debug_print "${FUNCNAME[0]}: unexpected call"
        exit_entryp 1
    else
        host_os_info_file='/host/etc/os-release'

        exec_cmd "egrep \"^ID=|^VERSION_ID=|^RHEL_VERSION=\" ${host_os_info_file}"

        eval local $(cat ${host_os_info_file} | grep ^ID=)
        eval local $(cat ${host_os_info_file} | grep ^VERSION_ID=)
        eval local $(cat ${host_os_info_file} | grep ^RHEL_VERSION=)

        if [ "${ID}" = "rhcos" ]; then
            OPENSHIFT_VERSION=${VERSION_ID:-4.9}
        elif [ "${ID}" = "rhel" ]; then
            RHEL_VERSION=${VERSION_ID:-8.4}
        else
            eval local $(cat ${host_os_info_file} | grep ^VERSION_ID=)
            RHEL_VERSION=${VERSION_ID:-8.4}
        fi
        RHEL_MAJOR_VERSION=${RHEL_VERSION%%.*}

        [[ ! -z ${RHEL_MAJOR_VERSION} ]] && debug_print "RHEL major version: ${RHEL_MAJOR_VERSION}"
        [[ ! -z ${OPENSHIFT_VERSION} ]] && debug_print "OPENSHIFT major version: ${OPENSHIFT_VERSION}"
    fi
}

# Install the kernel modules header/builtin/order files
# and generate the kernel version string
function redhat_install_prerequisites() {
    debug_print "Function: ${FUNCNAME[0]}"

    timestamp_print "Enabling RHOCP and EUS RPM repos..."

    redhat_fetch_major_ver

    if [ -n "${OPENSHIFT_VERSION}" ]; then
        exec_cmd "dnf config-manager --set-enabled rhocp-${OPENSHIFT_VERSION}-for-rhel-${RHEL_MAJOR_VERSION}-${ARCH}-rpms || true"
        if ! dnf makecache --releasever=${RHEL_VERSION}; then
            exec_cmd "dnf config-manager --set-disabled rhocp-${OPENSHIFT_VERSION}-for-rhel-${RHEL_MAJOR_VERSION}-${ARCH}-rpms"
        fi
    else
        : # Do nothing; alternative to `true` to avoid trap when if evaluates to false
    fi

    timestamp_print "Installing dependencies"

    rt_substr="rt-"
    k_ver="not_set"
    releasever_str="--releasever=${RHEL_VERSION}"

    if [[ ! ${FULL_KVER} =~ rt ]]; then
        rt_substr=""
        k_ver=$FULL_KVER

        eus_available=("8.4" "8.6" "8.8" "9.0" "9.2" "9.4")

        if [[ " ${eus_available[@]} " =~ " $RHEL_VERSION " ]]
        then
          exec_cmd "dnf config-manager --set-enabled rhel-${RHEL_MAJOR_VERSION}-for-${ARCH}-baseos-eus-rpms"
        fi



        # Dependencies for RH 9.x changed, thus explict installation required
        exec_cmd "dnf -q -y ${releasever_str} install kernel-${FULL_KVER}"
        exec_cmd "dnf -q -y ${releasever_str} install kernel-headers-${FULL_KVER}"
	# note: we provide --allowerasing flag to allow install of possible conflict packages
	# like openssl-libs which is required for kernel-devel that coflicts with openssl-fips in container
        exec_cmd "dnf -q -y ${releasever_str} install kernel-devel-${FULL_KVER}" --allowerasing
        exec_cmd "dnf -q -y ${releasever_str} install kernel-core-${FULL_KVER}"

    else
        debug_print "RT kernel identified"
        releasever_str=""

        exec_cmd "cp /host/etc/yum.repos.d/redhat.repo /etc/yum.repos.d/"

        # RH9.X RT kernel name pattern, example: 5.14.0-362.13.1.el9_3.x86_64+rt
        if [[ ${FULL_KVER} =~ rt$ ]]; then
            k_ver="${FULL_KVER%.*}.${ARCH}"

        elif [[ ${FULL_KVER} =~ rt ]]; then
            # RH8.X RT kernel name pattern, example: 4.18.0-513.11.1.rt7.313.el8_9.x86_64
            k_ver=$FULL_KVER

        else
            debug_print "Failed to identify RT kernel naming patter in kernel sring: ${FULL_KVER}"
            exit_entryp 1
        fi
    fi

    exec_cmd "dnf -q -y ${releasever_str} install kernel-${rt_substr}devel-${k_ver}"
    exec_cmd "dnf -q -y ${releasever_str} install kernel-${rt_substr}modules-${k_ver}"

    exec_cmd "dnf -q -y --releasever=${RHEL_VERSION} install elfutils-libelf-devel kernel-rpm-macros numactl-libs lsof rpm-build patch hostname"

    if ! dnf makecache --releasever=${RHEL_VERSION}; then
        exec_cmd "dnf config-manager --set-disabled rhel-${RHEL_MAJOR_VERSION}-for-${ARCH}-baseos-eus-rpms"
    fi
}

function set_append_driver_build_flags() {
    debug_print "Function: ${FUNCNAME[0]}"

    if [[ "${ENABLE_NFSRDMA}" = false ]]; then
        append_driver_build_flags="$append_driver_build_flags --without-mlnx-nfsrdma${pkg_dkms_suffix} --without-mlnx-nvme${pkg_dkms_suffix}"
    fi
}

function dtk_ocp_setup_driver_build() {
    debug_print "Function: ${FUNCNAME[0]}"

    timestamp_print "Copy required files to shared dir with OCP DTK"
    exec_cmd "cp -r ${NVIDIA_NIC_DRIVER_PATH} ${DTK_OCP_NIC_SHARED_DIR}/"

    exec_cmd "sed -i '/append_driver_build_flags=/c\append_driver_build_flags=\"${append_driver_build_flags}\"' ${DTK_OCP_BUILD_SCRIPT}"
    exec_cmd "sed -i '/DTK_OCP_COMPILED_DRIVER_VER=/c\DTK_OCP_COMPILED_DRIVER_VER=${NVIDIA_NIC_DRIVER_VER}' ${DTK_OCP_BUILD_SCRIPT}"
    exec_cmd "sed -i '/DTK_OCP_START_COMPILE_FLAG=/c\DTK_OCP_START_COMPILE_FLAG=${DTK_OCP_START_COMPILE_FLAG}' ${DTK_OCP_BUILD_SCRIPT}"
    exec_cmd "sed -i '/DTK_OCP_DONE_COMPILE_FLAG=/c\DTK_OCP_DONE_COMPILE_FLAG=${DTK_OCP_DONE_COMPILE_FLAG}' ${DTK_OCP_BUILD_SCRIPT}"

    exec_cmd "cp ${DTK_OCP_BUILD_SCRIPT} ${DTK_OCP_NIC_SHARED_DIR}/"

    exec_cmd "touch ${DTK_OCP_START_COMPILE_FLAG}"
    debug_print "Start compile flag is set at: ${DTK_OCP_START_COMPILE_FLAG}"
}

function dtk_ocp_finalize_driver_build() {
    debug_print "Function: ${FUNCNAME[0]}"

    rpms_path="${DTK_OCP_NIC_SHARED_DIR}/MLNX_OFED_SRC-${NVIDIA_NIC_DRIVER_VER}/RPMS/redhat-release-*/${ARCH}/"

    debug_print "Storing driver rpms from ${rpms_path} to ${driver_inventory_path}"
    exec_cmd "cp -rf ${rpms_path}/*.rpm ${driver_inventory_path}/"
}

function build_driver_from_src() {
    debug_print "Function: ${FUNCNAME[0]}"

    sub_path_str="RPMS"
    os_str="redhat"
    package_type="rpm"
    append_driver_build_flags="$append_driver_build_flags --disable-kmp"

    if ${IS_OS_UBUNTU}; then
        sub_path_str="DEBS"
        os_str="ubuntu"
        package_type="deb"
        append_driver_build_flags="$append_driver_build_flags --without-dkms"
    fi

    timestamp_print "Starting driver build"
    exec_cmd "${NVIDIA_NIC_DRIVER_PATH}/install.pl --without-depcheck --kernel ${FULL_KVER} --kernel-only --build-only --with-mlnx-tools --without-knem${pkg_dkms_suffix} --without-iser${pkg_dkms_suffix} --without-isert${pkg_dkms_suffix} --without-srp${pkg_dkms_suffix} --without-kernel-mft${pkg_dkms_suffix} --without-mlnx-rdma-rxe${pkg_dkms_suffix} ${append_driver_build_flags}"

    # If build from src triggered driver was not previously built for current kernel
    if [ ${reuse_driver_inventory} ]; then
        exec_cmd "cp ${NVIDIA_NIC_DRIVER_PATH}/${sub_path_str}/${os_str}*/${ARCH}/*.${package_type} ${driver_inventory_path}"

        timestamp_print "Stored driver packages at ${driver_inventory_path}"
    fi
}

function fix_src_link() {
    debug_print "Function: ${FUNCNAME[0]}"

    local target=$(readlink /usr/src/ofa_kernel/default)
    if [[ -e /usr/src/ofa_kernel/${ARCH}/${FULL_KVER} ]] && [[ -L /usr/src/ofa_kernel/default ]] && [[ "${target:0:1}" = / ]]; then
        exec_cmd "ln -snf ${ARCH}/${FULL_KVER} /usr/src/ofa_kernel/default"
    fi
}

function create_udev_rules() {
    debug_print "Function: ${FUNCNAME[0]}"
    # OFED deprecated old rules: 82-net-setup-link.rules, vf-net-link-name.sh
    # Script will create new udev rules file, with higher hierarchy, considering upgrade from older to newer OFED version

    # Method triggered if naming schema is short, creates udev rules file to engage and cut "np[n]" once device added with new driver

    echo "ACTION!=\"add\", GOTO=\"mlnx_ofed_name_end\"
SUBSYSTEM!=\"net\", GOTO=\"mlnx_ofed_name_end\"

# Rename physical interfaces (first case) of virtual functions (second case).
# Example names:
# enp8s0f0np0 -> enp8s0f0
# enp8s0f0np1v12 -> enp8s0f0v12

DRIVERS==\"mlx5_core\", ENV{ID_NET_NAME_PATH}!=\"\", \\
PROGRAM=\"/bin/sh -c 'echo \$env{ID_NET_NAME_PATH} | sed -r -e s/np[01]$// -e s/np[01]v/v/'\", \\
        ENV{ID_NET_NAME_PATH}=\"\$result\"

DRIVERS==\"mlx5_core\", ENV{ID_NET_NAME_SLOT}!=\"\", \\
PROGRAM=\"/bin/sh -c 'echo \$env{ID_NET_NAME_SLOT} | sed -r -e s/np[01]$// -e s/np[01]v/v/'\", \\
        ENV{ID_NET_NAME_SLOT}=\"\$result\"

LABEL=\"mlnx_ofed_name_end\"
" > ${MLX_UDEV_RULES_FILE}

    debug_print "Short interface name exposed by old driver, injected udev rules file ${MLX_UDEV_RULES_FILE}"
    debug_print `cat ${MLX_UDEV_RULES_FILE}`
}

function ubuntu_sync_network_configuration_tools() {
    debug_print "Function: ${FUNCNAME[0]}"

    # As part of openibd restart, mlnx_interface_mgr.sh is running and trying to read
    # /etc/network/interfaces file in case ifup exists and netplan doesn't.
    # In case the host doesn't include ifup but the container do we will fail on reading this file
    # and restarting openibd.
    # The container need to work on both cases where the host includes ifup and when it doesn't.
    # In order to support it we will install both ifup and netplan in the container and on run time
    # we will try to read /etc/network/interfaces (which is mounted from host) and if not exist
    # assume that ifup is missing in the host, in such case we will rename the ifup file in the
    # container so that mlnx_interface_mgr.sh will not find it and won't be trying to read missing
    # /etc/network/interfaces file.
    if [[ -e /etc/network/interfaces ]]; then
        timestamp_print "/etc/network/interfaces wasn't found, renaming ifup file (/sbin/ifup -> /sbin/ifup.bk)."
        if [[ -e /sbin/ifup ]]; then
            exec_cmd mv /sbin/ifup /sbin/ifup.bk
        fi
        return 0
    fi
}

function unload_storage_modules() {
    debug_print "Function: ${FUNCNAME[0]}"

    # Update openibd to unload storage modules
    debug_print "Extending modules unload list with storage modules"
    sed -i -e '/^UNLOAD_MODULES="[a-z]/aUNLOAD_MODULES="$UNLOAD_MODULES ib_isert nvme_rdma nvmet_rdma rpcrdma xprtrdma ib_srpt"' /etc/init.d/openibd

    if [ `grep ib_isert /etc/init.d/openibd -c` -lt 1 ]; then
        timestamp_print "Failed to inject storage modules for unload"
        exit_entryp 1
    fi
}

function restart_driver() {
    debug_print "Function: ${FUNCNAME[0]}"

    # Ensure mlx5_core dependencies loaded
    exec_cmd "modprobe -d /host tls"
    exec_cmd "modprobe -d /host psample"

    # Check if mlx5_ib depends on macsec if so, load it.
    if modinfo -Fdepends mlx5_ib | grep -qw macsec; then
        exec_cmd "modprobe -d /host macsec"
    fi

    load_pci_hyperv_intf=false

    # ARM does not contain relevant packages, also not a blocker for this OS type for mlx5_core load
    if [ "${ARCH}" != "aarch64" ]; then
        if ! ${IS_OS_UBUNTU}; then
            redhat_fetch_major_ver
            [[ $RHEL_MAJOR_VERSION -ge $RH_RT_MIN_MAJOR_VER ]] && load_pci_hyperv_intf=true
        else
            load_pci_hyperv_intf=true
        fi
    fi

    ${load_pci_hyperv_intf} && exec_cmd "modprobe -d /host pci-hyperv-intf"

    ${UNLOAD_STORAGE_MODULES} && unload_storage_modules

    exec_cmd "/etc/init.d/openibd restart"
}

function restructure_guid() {
    sysfs_guid_val=$1
    #Example: 0000:0000:0000:0000
    merged_guid=${sysfs_guid_val//:}
    #Example: 0000000000000000
    splitted_new=$(sed 's/../&:/g' <<< $merged_guid | cut -d: -f 1-8)
    #Example 00:00:00:00:00:00:00:00

    echo $splitted_new
}

function get_first_mlx_netdev_name() {
    for netdev_path in /sys/class/net/*; do
        if (grep $VENDOR "${netdev_path}"/device/vendor >/dev/null 2>&1); then
            netdev_name=$(basename "$netdev_path")
            break
        fi
    done
    echo ${netdev_name}
}

function find_mlx_devs() {
    debug_print "Function: ${FUNCNAME[0]}"

    for netdev_path in /sys/class/net/*; do
        if (grep $VENDOR "${netdev_path}"/device/vendor >/dev/null 2>&1); then
            debug_print "Fetching device info for: $netdev_path"

            pci_addr=$(basename $(readlink ${netdev_path}/device)) && [[ -n $pci_addr ]] || return 1
            dev_name=$(basename "$netdev_path") && [[ -n $dev_name ]] || return 1
            dev_operstate=$(cat "$netdev_path"/operstate) && [[ -n $dev_operstate ]] || return 1
            dev_mtu=$(cat "$netdev_path"/mtu) && [[ -n $dev_mtu ]] || return 1
            dev_type=""
            dev_guid=""

            if [[ -e "$netdev_path"/device/sriov_numvfs ]]; then
                pf_numvfs=$(cat "$netdev_path"/device/sriov_numvfs) && [[ -n $pf_numvfs ]] || return 1
            else
                pf_numvfs=0
            fi

            if [[ "$dev_name" =~ ^ib.* ]]; then
                dev_type="ib"
                sysfs_guid=$(cat ${netdev_path}/device/infiniband/*/node_guid) && [[ -n $sysfs_guid ]] || return 1
                dev_guid=$(restructure_guid $sysfs_guid)
            else
                dev_type="eth"
                dev_guid="-"
            fi

            eswitch_mode=$(devlink dev eswitch show pci/$pci_addr 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i == "mode") {print $(i+1); exit}}')

            if [ "${eswitch_mode}" == "switchdev" ]; then
                representor_regex="^pf([0-9]+)vf([0-9]+)$"

                mlnx_dev_phys_port_name=$(cat /sys/class/net/"${dev_name}"/phys_port_name 2>/dev/null)
                # Use regex to parse the string and extract numbers
                if [[ "$mlnx_dev_phys_port_name" =~ $representor_regex ]]; then
                    debug_print "Device ${mlnx_dev_name} is a switchdev representor, not a PF, skipping"

                    continue
                fi
            fi

            dev_record="$pci_addr $dev_type $dev_name $dev_operstate $dev_mtu $pf_numvfs $dev_guid $eswitch_mode"
            debug_print "Storing device record [${mlx_dev_record_idx}] $dev_record"
            mlx_devs_arr[${mlx_dev_record_idx}]=$dev_record
            # Example:
            # pci_addr      dev_type dev_name dev_operstate dev_mtu pf_numvfs dev_guid (For IB only) eswitch_mode
            # 0000:08:00.0  eth      eth2     up            1500    4         0c42:a103:0016:054c    legacy
            # 0000:08:00.1  eth      eth3     up            1500    0         0c42:a103:0016:054d    switchdev

            mlx_dev_record_idx=$((mlx_dev_record_idx+1))

            # Due to various interface namings between onboard and std. pci devices we must loop through all NVIDIA NICs
            #   interface names and sample if ANY contains np[n] suffix in ID_NET_NAME_PATH query
            # IF found long naming scheme - no need to inject udev rules
            if ! ${found_long_mlx_dev_id_net_name_path}; then
                mlx_dev_id_net_name_path=$(udevadm info --query=property ${netdev_path} | grep ID_NET_NAME_PATH | cut -d= -f 2)
                # udevadm from version 252 has straight-forward query: udevadm info --query=property --property=ID_NET_NAME_PATH --value /sys/class/net/<iface_name>
                # Ubuntu 22.04 built-in udevadm version is 249, which lacks filtering query by property name, thus utilizing genetic piped commands

                debug_print "Sampling interface for ID_NET_NAME_PATH: ${netdev_path} => ${mlx_dev_id_net_name_path}"
                if [[ ${mlx_dev_id_net_name_path} =~ np[0-3]$ ]]; then
                    debug_print "Old driver exposed long (new) interface name scheme, udev rules will not be injected"
                    found_long_mlx_dev_id_net_name_path=true
                fi
            fi
        fi
    done
}

function find_mlx_vfs() {
    debug_print "Function: ${FUNCNAME[0]}"

    num_mlx_devices=${mlx_dev_record_idx-1}
    timestamp_print "Query VFs info from [${num_mlx_devices}] devices"

    while [ ${num_mlx_devices} -gt 0 ]; do
        declare -a mlx_dev_info=(${mlx_devs_arr[$((num_mlx_devices-1))]})

        # pci_addr dev_type dev_name dev_operstate dev_mtu pf_numvfs dev_guid eswitch_mode
        # [0]      [1]      [2]      [3]           [4]     [5]       [6]      [7]
        mlnx_dev_pci=${mlx_dev_info[0]}
        mlnx_dev_type=${mlx_dev_info[1]}
        mlnx_dev_name=${mlx_dev_info[2]}
        mlnx_dev_numvfs=${mlx_dev_info[5]}
        mlnx_dev_eswitch_mode=${mlx_dev_info[7]}

        if [ "${mlnx_dev_numvfs}" == "0" ]; then
            debug_print "Device ${mlnx_dev_name} does not have open VFs, skipping"
            num_mlx_devices=$(($num_mlx_devices-1))

            continue
        fi

        found_sriov_conf=$(($found_sriov_conf+1))

        debug_print "Fetching [$mlnx_dev_numvfs] VFs info for PCI device: ${mlnx_dev_pci} ($mlnx_dev_name)"

        for vf_index in $(seq 0 1 $((mlnx_dev_numvfs-1))); do
            vf_dev_base_path="/sys/class/net/$mlnx_dev_name/device/virtfn$vf_index/net/"
            vf_name=$(ls "$vf_dev_base_path") && [[ -n $vf_name ]] || return 1
            vf_netdev_path=${vf_dev_base_path}${vf_name}

            debug_print "Fetching VF device info for: $vf_netdev_path"
            vf_ip_link_json=$(ip -j link show $mlnx_dev_name | jq -r .[0].vfinfo_list[$vf_index]) && [[ -n $vf_ip_link_json ]] || return 1

            vf_operstate=$(cat "$vf_netdev_path"/operstate) && [[ -n $vf_operstate ]] || return 1
            vf_mac=$(cat "$vf_netdev_path"/address) && [[ -n $vf_mac ]] || return 1
            vf_admin_mac=$(echo ${vf_ip_link_json} | jq -r .address) && [[ -n $vf_admin_mac ]] || return 1
            vf_pci_addr=$(basename $(readlink "$vf_netdev_path"/device)) && [[ -n $vf_pci_addr ]] || return 1
            vf_mtu_val=$(cat "$vf_netdev_path"/mtu) && [[ -n $vf_mtu_val ]] || return 1

            if [ "$mlnx_dev_type" == "ib" ]; then
                vf_guid=$(echo ${vf_ip_link_json} | jq -r '."port guid"') && [[ -n $vf_guid ]] || return 1
            else
                vf_guid="-"
            fi

            vf_record="$mlnx_dev_pci $vf_pci_addr $mlnx_dev_name $vf_name $mlnx_dev_type $vf_index $vf_operstate $vf_mac $vf_admin_mac $vf_mtu_val $vf_guid"
            debug_print "Storing VF record [${vf_record_idx}]: $vf_record"
            mlx_vfs_arr[$vf_record_idx]="$vf_record"
            # Example:
            # pf_pci_addr  vf_pci_addr  mlnx_dev_name vf_name mlnx_dev_type vf_index vf_operstate vf_mac            vf_admin_mac      vf_mtu_val vf_guid
            # 0000:08:00.0 0000:08:00.2 eth2          eth6    eth           0        down         2e:20:07:8a:7f:7c a1:01:cc:dd:ee:ff 1500       50:25:e1:32:af:a2:b8:a0

            vf_record_idx=$((vf_record_idx+1))
        done

        num_mlx_devices=$(($num_mlx_devices-1))
    done
}

function find_switchdev_representors() {
    debug_print "Function: ${FUNCNAME[0]}"
    num_mlx_devices=${mlx_dev_record_idx-1}
    timestamp_print "Query representors info from [${num_mlx_devices}] devices"

    while [ ${num_mlx_devices} -gt 0 ]; do
        declare -a mlx_dev_info=(${mlx_devs_arr[$((num_mlx_devices-1))]})

        # pci_addr dev_type dev_name dev_operstate dev_mtu pf_numvfs dev_guid eswitch_mode
        # [0]      [1]      [2]      [3]           [4]     [5]       [6]      [7]
        mlnx_dev_pci=${mlx_dev_info[0]}
        mlnx_dev_name=${mlx_dev_info[2]}
        mlnx_dev_numvfs=${mlx_dev_info[5]}
        mlnx_dev_eswitch_mode=${mlx_dev_info[7]}

        if [ "${mlnx_dev_eswitch_mode}" != "switchdev" ]; then
            debug_print "Device ${mlnx_dev_name} is not in switchdev mode, skipping"

            num_mlx_devices=$(($num_mlx_devices-1))
            continue
        fi

        regex="^p([0-9]+)$"
        mlnx_dev_phys_port_name=$(cat /sys/class/net/"${mlnx_dev_name}"/phys_port_name 2>/dev/null)
        if [[ "$mlnx_dev_phys_port_name" =~ $regex ]]; then
            mlnx_dev_phys_port_num="${BASH_REMATCH[1]}"
        else
            debug_print "Device ${mlnx_dev_name} is not a PF, skipping"

            num_mlx_devices=$(($num_mlx_devices-1))
            continue
        fi

        mlnx_dev_phys_switch_id=$(cat /sys/class/net/"${mlnx_dev_name}"/phys_switch_id 2>/dev/null)

        debug_print "Device phys port num ${mlnx_dev_phys_port_num} check"
        debug_print "Device phys port name ${mlnx_dev_phys_port_name} check"

        if [ "${mlnx_dev_numvfs}" == "0" ]; then
            debug_print "Device ${mlnx_dev_name} does not have open VFs, skipping"

            num_mlx_devices=$(($num_mlx_devices-1))
            continue
        fi

        debug_print "Fetching [$mlnx_dev_numvfs] representors info for PCI device: ${mlnx_dev_pci} ($mlnx_dev_name)"

        for netdev_path in /sys/class/net/"${mlnx_dev_name}"/subsystem/*; do
            dev_phys_port_name=$(cat "${netdev_path}"/phys_port_name 2>/dev/null)

            regex="^pf([0-9]+)vf([0-9]+)$"

            # Use regex to parse the string and extract numbers
            if [[ "$dev_phys_port_name" =~ $regex ]]; then
                dev_pf_port_number="${BASH_REMATCH[1]}"
                dev_vf_id_number="${BASH_REMATCH[2]}"
            else
                debug_print "Device ${netdev_path} is not a switchdev representor, skipping"
                continue
            fi

            if [ "${mlnx_dev_phys_switch_id}" != "$(cat "${netdev_path}"/phys_switch_id 2>/dev/null)" ]; then
                debug_print "Physical switch id of device ${netdev_path} does not match PF ${mlnx_dev_name}, skipping"
                continue
            fi

            if [ "${dev_pf_port_number}" != "${mlnx_dev_phys_port_num}" ]; then
                debug_print "Switchdev representor ${netdev_path} does not belong to this PF ${mlnx_dev_name}"
                continue
            fi

            representor_name=$(basename "$netdev_path") && [[ -n $representor_name ]] || return 1
            representor_operstate=$(cat "$netdev_path"/operstate) && [[ -n $representor_operstate ]] || return 1
            representor_mtu_val=$(cat "$netdev_path"/mtu) && [[ -n $representor_mtu_val ]] || return 1

            representor_record="$mlnx_dev_phys_switch_id $mlnx_dev_phys_port_num $dev_vf_id_number $representor_name $representor_operstate $representor_mtu_val"
            debug_print "Storing switchdev representor record [${representor_record_idx}]: $representor_record"
            switchdev_representors_arr[$representor_record_idx]="$representor_record"
            # Example:
            # pf_phys_switch_id pf_port_num vf_id representor_name representor_operstate representor_mtu_val
            # 8a45730003da341c  1           3     enp3s1f1npf1vf3  up                    1500

            representor_record_idx=$((representor_record_idx+1))
        done

        num_mlx_devices=$(($num_mlx_devices-1))
    done
}

function restore_sriov_config() {
    debug_print "Function: ${FUNCNAME[0]}"

    if [ $found_sriov_conf -eq 0 ]; then
        debug_print "SR-IOV config not found"
        return 0
    fi

    num_mlx_devices=${mlx_dev_record_idx-1}
    debug_print "Query VFs info from [${num_mlx_devices}] devices ($vf_record_idx VFs)"

    while [ ${num_mlx_devices} -gt 0 ]; do
        declare -a mlx_dev_info=(${mlx_devs_arr[$((num_mlx_devices-1))]})

        # pci_addr dev_type dev_name dev_operstate dev_mtu pf_numvfs dev_guid (For IB only) eswitch_mode
        # [0]      [1]      [2]      [3]           [4]     [5]       [6]                    [7]

        pf_pci_addr=${mlx_dev_info[0]}
        pf_type=${mlx_dev_info[1]}
        pf_old_name=${mlx_dev_info[2]}
        pf_oper_state=${mlx_dev_info[3]}
        pf_mtu=${mlx_dev_info[4]}
        pf_numvfs=${mlx_dev_info[5]}
        pf_eswitch_mode=${mlx_dev_info[7]}

        if [ "${pf_numvfs}" == "0" ]; then
            num_mlx_devices=$(($num_mlx_devices-1))
            continue
        fi

        pf_pci_dev_path="/sys/bus/pci/devices/${pf_pci_addr}"
        pf_new_dev_name=$(ls "$pf_pci_dev_path/net/" | head -n 1)
        pf_new_netdev_path="${pf_pci_dev_path}/net/${pf_new_dev_name}"

        timestamp_print "Restoring SR-IOV config for device: $pf_pci_addr (VFs=$pf_numvfs) netdev name: ${pf_new_dev_name}"

        # To support the old kernel versions, we need to follow the recommended way of creating switchdev VFs
        # 1) Set the NIC in legacy mode
        # 2) Create the required amount of VFs
        # 3) Unbind all of the VFs
        # 4) Set the NIC in switchdev mode
        if [ "${pf_eswitch_mode}" == "switchdev" ]; then
            exec_cmd "devlink dev eswitch set pci/$pf_pci_addr mode legacy"
        fi

        exec_cmd "ip link set dev ${pf_new_dev_name} $pf_oper_state"
        # Permission denied via sysfs

        exec_cmd "echo ${pf_numvfs} >> ${pf_pci_dev_path}/sriov_numvfs"

        for vf_record in $(seq 0 1 $((vf_record_idx-1))); do
            declare -a mlx_vf_info=(${mlx_vfs_arr[$vf_record]})

            vf_parent_pci_addr=${mlx_vf_info[0]}

            debug_print "Inspecting VF entry: ${mlx_vf_info[@]}"

            if [ "${vf_parent_pci_addr}" == "${pf_pci_addr}" ]; then
                # pf_pci_addr vf_pci_addr dev_name vf_name mlnx_dev_type vf_index vf_operstate vf_mac vf_admin_mac vf_mtu_val vf_guid
                # [0]         [1]         [2]      [3]     [4]           [5]      [6]          [7]    [8]          [9]        [10]
                vf_pci_addr=${mlx_vf_info[1]}
                vf_name=${mlx_vf_info[3]}
                vf_type=${mlx_vf_info[4]}
                vf_index=${mlx_vf_info[5]}
                vf_operstate=${mlx_vf_info[6]}
                vf_mac=${mlx_vf_info[7]}
                vf_admin_mac=${mlx_vf_info[8]}
                vf_mtu=${mlx_vf_info[9]}
                vf_guid=${mlx_vf_info[10]}

                timestamp_print "Restoring VF configuration for device: $vf_pci_addr"

                vf_pci_dev_path="/sys/bus/pci/devices/$vf_pci_addr"
                vf_new_dev_name=$(ls "$vf_pci_dev_path"/net/)

                vf_new_netdev_path="${vf_pci_dev_path}/net/${vf_new_dev_name}"

                if [ "${vf_type}" == "ib" ]; then
                    if [ "$vf_guid" != "00:00:00:00:00:00:00:00" ]; then
                        # Do not set invalid GUID
                        exec_cmd "ip link set $pf_new_dev_name vf $vf_index port_guid $vf_guid"
                        exec_cmd "ip link set $pf_new_dev_name vf $vf_index node_guid $vf_guid"
                    else
                        debug_print "VF GUID not set (00:00:00:00:00:00:00:00), skipping"
                    fi
                else
                    exec_cmd "ip link set ${vf_new_dev_name} address ${vf_mac}"
                    exec_cmd "ip link set dev ${pf_new_dev_name} vf ${vf_index} mac ${vf_admin_mac}"
                    # Permission denied via sysfs
                fi

                 # Driver rebind VF
                exec_cmd "echo ${vf_pci_addr} > ${DRIVER_PATH}/unbind"

                # As per the comment above, we need to set the NIC in switchdev mode first before binding back the VFs
                if [ "${pf_eswitch_mode}" == "switchdev" ]; then
                    continue
                fi

                exec_cmd "echo ${vf_pci_addr} > ${DRIVER_PATH}/bind"

                sleep ${BIND_DELAY_SEC}

                exec_cmd "echo $vf_mtu > $vf_new_netdev_path/mtu"
                exec_cmd "ip link set dev ${vf_new_dev_name} $vf_operstate"
                # Permission denied via sysfs
            fi
        done

        # Set the PF in switchdev mode and rebind its VFs to the driver
        if [ "${pf_eswitch_mode}" == "switchdev" ]; then
            exec_cmd "devlink dev eswitch set pci/$pf_pci_addr mode switchdev"

            for vf_record in $(seq 0 1 $((vf_record_idx-1))); do
                declare -a mlx_vf_info=(${mlx_vfs_arr[$vf_record]})

                vf_parent_pci_addr=${mlx_vf_info[0]}

                debug_print "Inspecting VF entry: ${mlx_vf_info[@]}"

                if [ "${vf_parent_pci_addr}" == "${pf_pci_addr}" ]; then
                    # pf_pci_addr vf_pci_addr dev_name vf_name mlnx_dev_type vf_index vf_operstate vf_mac vf_admin_mac vf_mtu_val vf_guid
                    # [0]         [1]         [2]      [3]     [4]           [5]      [6]          [7]    [8]          [9]        [10]
                    vf_pci_addr=${mlx_vf_info[1]}
                    vf_operstate=${mlx_vf_info[6]}
                    vf_mtu=${mlx_vf_info[9]}

                    timestamp_print "Restoring VF configuration for switchdev device: $vf_pci_addr"

                    exec_cmd "echo ${vf_pci_addr} > ${DRIVER_PATH}/bind"

                    sleep ${BIND_DELAY_SEC}

                    vf_pci_dev_path="/sys/bus/pci/devices/$vf_pci_addr"
                    vf_new_dev_name=$(ls "$vf_pci_dev_path"/net/)

                    vf_new_netdev_path="${vf_pci_dev_path}/net/${vf_new_dev_name}"

                    exec_cmd "echo $vf_mtu > $vf_new_netdev_path/mtu"
                    exec_cmd "ip link set dev ${vf_new_dev_name} $vf_operstate"
                    # Permission denied via sysfs
                fi
            done

        fi

        exec_cmd "echo $pf_mtu > $pf_new_netdev_path/mtu"

        num_mlx_devices=$(($num_mlx_devices-1))
    done

    for representor_record in $(seq 0 1 $((representor_record_idx-1))); do
        declare -a representor_info=(${switchdev_representors_arr[$representor_record]})

        debug_print "Inspecting switchdev representor entry: ${representor_info[@]}"

        # pf_phys_switch_id pf_port_num vf_id representor_name representor_operstate representor_mtu_val
        # [0]               [1]         [2]   [3]              [4]                   [5]
        pf_phys_switch_id=${representor_info[0]}
        pf_port_num=${representor_info[1]}
        vf_id=${representor_info[2]}
        representor_name=${representor_info[3]}
        representor_operstate=${representor_info[4]}
        representor_mtu_val=${representor_info[5]}

        mlnx_dev_phys_switch_id=$(cat /sys/class/net/"${mlnx_dev_name}"/phys_switch_id 2>/dev/null)

        for netdev_path in /sys/class/net/*; do
            if [ "${pf_phys_switch_id}" != "$(cat "${netdev_path}"/phys_switch_id 2>/dev/null)" ]; then
                debug_print "Physical switch id of device ${netdev_path} does not match the record, skipping"
                continue
            else
                debug_print "Physical switch id of device ${netdev_path} matches the record"
            fi

            dev_phys_port_name=$(cat "${netdev_path}"/phys_port_name 2>/dev/null)

            regex="^pf([0-9]+)vf([0-9]+)$"

            # Use regex to parse the string and extract numbers
            if [[ "$dev_phys_port_name" =~ $regex ]]; then
                dev_pf_port_number="${BASH_REMATCH[1]}"
                dev_vf_id_number="${BASH_REMATCH[2]}"
            else
                debug_print "Device ${netdev_path} is not a switchdev representor, skipping"
                continue
            fi

            if [ "${dev_pf_port_number}" != "${pf_port_num}" ]; then
                debug_print "Switchdev representor ${netdev_path} does not belong to this port ${pf_port_num}"
                continue
            fi

            if [ "${dev_vf_id_number}" != "${vf_id}" ]; then
                debug_print "Switchdev representor ${netdev_path} does not belong to this vf id ${vf_id}"
                continue
            fi

            debug_print "Switchdev representor ${netdev_path} belongs to this vf id ${vf_id}, setting parameters"

            representor_new_name=$(basename "$netdev_path") && [[ -n $representor_new_name ]]

            exec_cmd "ip link set dev ${representor_new_name} name ${representor_name}"
            exec_cmd "ip link set dev ${representor_name} mtu ${representor_mtu_val}"
            exec_cmd "ip link set dev ${representor_name} ${representor_operstate}"

            break
        done

    done
}

function store_devices_conf() {
    debug_print "Function: ${FUNCNAME[0]}"

    mlx_dev_record_idx=0
    vf_record_idx=0
    representor_record_idx=0
    found_sriov_conf=0

    if ${mlx5_core_loaded}; then
        find_mlx_devs
        find_mlx_vfs
        find_switchdev_representors
    else
        debug_print "Driver not loaded, skipped store netdev conf info"
    fi
}

function load_nfsrdma() {
    debug_print "Function: ${FUNCNAME[0]}"

    if [[ "${ENABLE_NFSRDMA}" = true ]]; then
        exec_cmd "modprobe nvme"
        exec_cmd "modprobe nvme-rdma"
        exec_cmd "modprobe rpcrdma"
    fi
}

# function check_loaded_kmod_srcver_vs_modinfo() returns 0 if all provided modules
# srcversion from sysfs match information from modinfo
function check_loaded_kmod_srcver_vs_modinfo() {
    debug_print "Function: ${FUNCNAME[0]}"

    for module in "$@"; do
        debug_print "checking module $module"
        local srcver_from_modinfo=$(/sbin/modinfo $module 2>/dev/null | grep srcversion | awk '{print $NF}')
        local srcver_from_sysfs=$(/bin/cat /sys/module/$module/srcversion)
        debug_print "module: $module, srcver_from_modinfo: $srcver_from_modinfo, srcver_from_sysfs: $srcver_from_sysfs"
        if [[ "${srcver_from_modinfo}" != "${srcver_from_sysfs}" ]]; then
            debug_print "module: $module, srcver differs"
            return 1
        fi
    done
    # all modules srcver match between modinfo and sysfs
    return 0
}

function load_driver() {
    debug_print "Function: ${FUNCNAME[0]}"

    if ! check_loaded_kmod_srcver_vs_modinfo mlx5_core mlx5_ib; then
        restart_driver

        new_driver_loaded=true

        restore_sriov_config
        load_nfsrdma

    else
        debug_print "Loaded and candidate drivers are identical, *skipping reload*"
        new_driver_loaded=true
    fi

    print_loaded_drv_ver_str

    mount_rootfs

    set_driver_readiness 1
}

function install_driver() {
    debug_print "Function: ${FUNCNAME[0]}"

    # Prevent depmod from giving a WARNING about missing files during installation
    [[ ! -d "/lib/modules/${FULL_KVER}" ]] && exec_cmd "mkdir /lib/modules/${FULL_KVER}"

    exec_cmd "touch /lib/modules/${FULL_KVER}/modules.order"
    exec_cmd "touch /lib/modules/${FULL_KVER}/modules.builtin"

    if ${IS_OS_UBUNTU}; then
        exec_cmd "dpkg -i ${driver_inventory_path}/*.deb"

    else
        exec_cmd "rpm -ivh --replacepkgs --nodeps ${driver_inventory_path}/*.rpm"
    fi

    # Introduce installed kernel modules
    exec_cmd "depmod ${FULL_KVER}"
}

function build_driver() {
    debug_print "Function: ${FUNCNAME[0]}"

    if ${reuse_driver_inventory}; then
        driver_inventory_path="${NVIDIA_NIC_DRIVERS_INVENTORY_PATH}/${FULL_KVER}/${NVIDIA_NIC_DRIVER_VER}"

        if [ -d "${driver_inventory_path}" ]; then
            timestamp_print "Skipping driver build, reusing previously built packages for kernel ${FULL_KVER}"
            return 0
        fi
    else
        driver_inventory_path=/tmp/nvidia_nic_driver_$(date +"%d-%m-%Y_%H-%M-%S")
    fi

    exec_cmd "mkdir -p ${driver_inventory_path}"
    debug_print "Driver modules dest path: ${driver_inventory_path}"

    driver_build_incomplete=true

    set_append_driver_build_flags

    # OCP DTK Build
    if [ "${DTK_OCP_DRIVER_BUILD}" == "true" ]; then
        DTK_OCP_DONE_COMPILE_FLAG="${DTK_OCP_DONE_COMPILE_FLAG_PREFIX}$(echo ${NVIDIA_NIC_DRIVER_VER} | sed 's/[.-]/_/g')"

        if [ ! -f "${DTK_OCP_DONE_COMPILE_FLAG}" ]; then

            dtk_ocp_setup_driver_build

            # Await driver build completion
            sleep_sec=300
            total_retries=10
            total_sleep_sec=0
            while [ ! -f "${DTK_OCP_DONE_COMPILE_FLAG}" ] && [ ${total_retries} -gt 0 ]; do
                timestamp_print "Awaiting openshift driver toolkit to complete NIC driver build, next query in ${sleep_sec} sec"
                sleep ${sleep_sec}
                let total_sleep_sec+=sleep_sec
                if [ $sleep_sec -gt 10 ]; then
                    sleep_sec=$((sleep_sec/2))
                fi
                total_retries=$((total_retries-1))
            done
            if [ ${total_retries} -eq 0 ]; then
                timestamp_print "Timeout (${total_sleep_sec} sec.) awating DTK compilation, ${DTK_OCP_DONE_COMPILE_FLAG} not found"
                exit_entryp 1
            fi
        fi

        dtk_ocp_finalize_driver_build

    # Standard build
    else
        if ${build_src}; then
            if ${IS_OS_UBUNTU}; then
                ubuntu_install_prerequisites
            else
                redhat_install_prerequisites
            fi

            build_driver_from_src

            fix_src_link
        fi
    fi

    driver_build_incomplete=false
}

function cleanup_driver_inventory() {
    debug_print "Function: ${FUNCNAME[0]}"

    kernel_ver_dirs=$(ls ${NVIDIA_NIC_DRIVERS_INVENTORY_PATH})
    for kernel_ver_dir in ${kernel_ver_dirs}; do

        if [ ${kernel_ver_dir} != ${FULL_KVER} ]; then
            exec_cmd "rm -rf ${NVIDIA_NIC_DRIVERS_INVENTORY_PATH}/${kernel_ver_dir}"
            continue
        fi

        driver_ver_dirs=$(ls ${NVIDIA_NIC_DRIVERS_INVENTORY_PATH}/${kernel_ver_dir})

        found_driver_ver_dirs=0
        removed_driver_ver_dirs=0
        for driver_ver_dir in ${driver_ver_dirs}; do
            found_driver_ver_dirs=$((${found_driver_ver_dirs}+1))

            if [ ${driver_ver_dir} != ${NVIDIA_NIC_DRIVER_VER} ]; then
                exec_cmd "rm -rf ${NVIDIA_NIC_DRIVERS_INVENTORY_PATH}/${kernel_ver_dir}/${driver_ver_dir}"
                removed_driver_ver_dirs=$((${removed_driver_ver_dirs}+1))
            fi
        done

        [[ ${found_driver_ver_dirs} -eq ${removed_driver_ver_dirs} ]] && exec_cmd "rm -rf ${NVIDIA_NIC_DRIVERS_INVENTORY_PATH}/${kernel_ver_dir}"

    done
}

function unload_blocking_modules() {
    debug_print "Function: ${FUNCNAME[0]}"

    timestamp_print "Verifying loaded modules will not prevent future driver restart"

    modules_sequence=('nvidia_peermem')
    for module in ${modules_sequence[@]}; do
        debug_print "Checking if module ${module} loaded"

        ref_cnt=$(lsmod | grep "^${module}" | awk '{print $3}')

        case "$ref_cnt" in
            "")
            debug_print "Module ${module} not loaded"
            ;;

            "0")
            timestamp_print "Attempting to unload (unutilized) module: ${module}"
            exec_cmd "rmmod ${module}"
            ;;

            *)
            timestamp_print "Module ${module} is used and will FAIL driver reload, aborting execution."
            exit_entryp 1
        esac
    done
}

function prepare_gcc() {
    debug_print "Function: ${FUNCNAME[0]}"

    ALT_GCC_PRIO=200
    proc_version=$(cat /proc/version)
    kernel_gcc_ver=$(echo ${proc_version} | grep -o 'gcc-[0-9]*')

    debug_print "/proc/version: ${proc_version}"

    if [ ! -z "${kernel_gcc_ver}" ]; then
        debug_print "Kernel compiled with ${kernel_gcc_ver}"

        exec_cmd "apt update; apt-get install ${kernel_gcc_ver}"
        exec_cmd "update-alternatives --install /usr/bin/gcc gcc /usr/bin/${kernel_gcc_ver} ${ALT_GCC_PRIO}"

        timestamp_print "Set ${kernel_gcc_ver} for driver compilation, matching kernel compiled version"
    fi
}

function print_loaded_drv_ver_str() {
    debug_print "Function: ${FUNCNAME[0]}"

    if $(lsmod | grep -i ^mlx5_core -q); then
        mlx_net_dev_name=$(get_first_mlx_netdev_name)
        mlx_dev_pci=$(basename $(readlink /sys/class/net/${mlx_net_dev_name}/device))

        debug_print "Fetching driver info via ethtool from netdev ${mlx_net_dev_name} (${mlx_dev_pci})"
        drv_ver_str=$(ethtool --driver ${mlx_net_dev_name} | grep ^version | cut -d" " -f 2)

        timestamp_print "Current mlx5_core driver version: ${drv_ver_str}"

        local loaded_drv_srcver=$(/bin/cat /sys/module/mlx5_core/srcversion)
        debug_print "Current mlx5_core driver srcversion: ${loaded_drv_srcver}"

    else
        timestamp_print "mlx5_core driver not loaded"
    fi
}

############################## Exec start #####################################
[[ -f "/tmp/entrypoint_done" ]] && echo "Entrypoint already executed, restart container" && exit 0

timestamp_print "NVIDIA driver container exec start"

ARCH=$(uname -m)
FULL_KVER=$(uname -r)
IS_OS_UBUNTU=true; [[ "$(grep -i ubuntu /etc/os-release -c)" == "0" ]] && IS_OS_UBUNTU=false
RHEL_MAJOR_VERSION=0
OPENSHIFT_VERSION=""

DTK_OCP_BUILD_SCRIPT="/root/dtk_nic_driver_build.sh"
DTK_OCP_START_COMPILE_FLAG=${DTK_OCP_NIC_SHARED_DIR}/dtk_start_compile
DTK_OCP_DONE_COMPILE_FLAG_PREFIX=${DTK_OCP_NIC_SHARED_DIR}/dtk_done_compile_
DTK_OCP_DONE_COMPILE_FLAG=""

VENDOR=0x15b3
DRIVER_PATH=/sys/bus/pci/drivers/mlx5_core
MLX_DRIVERS_MOUNT=/run/mellanox/drivers
MLX_UDEV_RULES_FILE=/host/etc/udev/rules.d/77-mlnx-net-names.rules
SHARED_KERNEL_HEADERS_DIR=/usr/src/
BIND_DELAY_SEC=3

DRIVER_READY_FILE=/run/mellanox/drivers/.driver-ready
new_driver_loaded=false

append_driver_build_flags=""
pkg_dkms_suffix=""
found_long_mlx_dev_id_net_name_path=false

mlx5_core_loaded=false; [[ "$(lsmod | grep -i ^mlx5_core -c)" != "0" ]] && mlx5_core_loaded=true
build_src=false
build_precompiled=false
reuse_driver_inventory=false
driver_inventory_path=""
driver_build_incomplete=false

RH_RT_MIN_MAJOR_VER=9

declare -a mlx_devs_arr
declare -a mlx_vfs_arr
declare -a switchdev_representors_arr
mlx_dev_record_idx=0

if ${IS_OS_UBUNTU}; then
    debug_print "OS is Ubuntu"
    pkg_dkms_suffix="-dkms"
else
    debug_print "OS is Red Hat"
fi

debug_print "[os-release]: "$(cat /etc/os-release)
debug_print "[uname -a]: "$(uname -a)
debug_print "[free -m]: "$(free -m)

if [ -z "${NVIDIA_NIC_DRIVER_VER}" ]; then
    timestamp_print "Mandatory container driver ver. ENV variable not set, terminating" && exit_entryp 1
fi

timestamp_print "Container full version: ${NVIDIA_NIC_DRIVER_VER}-${NVIDIA_NIC_CONTAINER_VER}"

unload_blocking_modules

storage_modules_loaded=$(lsmod | egrep 'ib_isert|nvme_rdma|nvmet_rdma|rpcrdma|xprtrdma|ib_srpt' -c)

if ! ${UNLOAD_STORAGE_MODULES} && [[ "${storage_modules_loaded}" != "0" ]]; then
    timestamp_print "Storage modules are loaded for current driver, terminating prior driver reload failure due to UNLOAD_STORAGE_MODULES not set to \"true\""
    exit_entryp 0
fi

# Identify execution type according entrypoint CMD argument injected by Dockerfile
case "$@" in
    "sources")
    timestamp_print "Executing driver sources container"

    if [ -z "${NVIDIA_NIC_DRIVER_PATH}" ]; then
        timestamp_print "Mandatory container driver sources path ENV variable not set, terminating"
        exit_entryp 1
    fi

    debug_print "Drivers sources path: ${NVIDIA_NIC_DRIVER_PATH}"

    prepare_gcc

    build_src=true

    if [[ ! -z ${NVIDIA_NIC_DRIVERS_INVENTORY_PATH} ]]; then
        timestamp_print "Drivers inventory path is set: ${NVIDIA_NIC_DRIVERS_INVENTORY_PATH}"
        if [ -d "${NVIDIA_NIC_DRIVERS_INVENTORY_PATH}" ]; then
            reuse_driver_inventory=true

        else
            timestamp_print "Drivers inventory folder unreachable"
            exit_entryp 1
        fi

    else
        timestamp_print "Driver inventory path not set, container will always recompile driver on startup"
    fi
    ;;

    "precompiled")
    timestamp_print "Executing precompiled driver container"
    build_precompiled=true
    ;;

    *)
    timestamp_print "Failed to determine container type, mandatory execution parameter not provided"
    exit_entryp 1
esac

# Unset driver readiness in case it was set in a previous run of this container
# and container was killed
set_driver_readiness 0

# Remove possible udev rules leftovers from ungraceful termination
delete_udev_rules

# Fetch devices configuration for restore after new driver loaded / removed
store_devices_conf

[ "${CREATE_IFNAMES_UDEV}" = true ] && [ $mlx_dev_record_idx -gt 0 ] && ! ${found_long_mlx_dev_id_net_name_path} && create_udev_rules

# Any command execution FAILURE from this point will terminate the container
trap "terminate_event" SIGSTOP SIGINT SIGTERM EXIT

if ! ${build_precompiled}; then
    build_driver

    install_driver

    ${IS_OS_UBUNTU} && ubuntu_sync_network_configuration_tools
fi

load_driver

${reuse_driver_inventory} && cleanup_driver_inventory

exec_cmd "touch /tmp/entrypoint_done"

timestamp_print "NVIDIA driver container exec end, sleeping"
sleep infinity & wait
