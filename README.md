# Precompiled Container Build Instructions for NVIDIA NIC Drivers

This document provides instructions on how to build container with NVIDIA NIC drivers, precompiled for **specific kernel version**.

Doca-driver container is designed to build and install NVIDIA NIC drivers as part of [NVIDIA Network Operator](https://github.com/Mellanox/network-operator?tab=readme-ov-file#nvidia-network-operator) solution.


## Prerequisites

Before you begin, ensure that you have the following prerequisites:

**Common**:
1. Docker (Ubuntu) / Podman (RH) installed on your build system.
2. Web access to NVIDIA NIC drivers sources[^1].


**RHEL**:
1. Active subscription and login credentials for `registry.redhat.io`[^2].

**RHCOS**:
1. `oc` CLI tool [^3].
2. OpenShift pull secret [^4].

## Dockerfile Overview

To build the precompiled container, the Dockerfile is constructed in a multistage fashion.  
This approach is used to optimize the resulting container image size and reduce the number of dependencies included in the final image.  

The Dockerfile consists of the following stages:

1. **Base Image Update**: The base image is updated and common requirements are installed. This stage sets up the basic environment for the subsequent stages.

2. **Download Driver Sources**: This stage downloads the Mellanox OFED driver sources to the specified path. It prepares the necessary files for the driver build process.

3. **Build Driver**: The driver is built using the downloaded sources and installed on the container. This stage ensures that the driver is compiled and configured correctly for the target system.

4. **Install precompiled driver**: Finally, the precompiled driver is installed on clean container. This stage sets up the environment to run the NVIDIA NIC drivers on the target system.

>[!IMPORTANT]
>For proper Network Operator functionality container tag name must be in following pattern:
>**driver_ver-container_ver-kernel_ver-os-arch**
>For example: 24.01-0.3.3.1-0-5.15.0-25-generic-ubuntu22.04-amd64

## Common _mandatory_ build parameters
Before building the container, you need to provide following parameters as `build-arg` for container build:
1. `D_OS`: The Linux distribution (e.g., ubuntu22.04 / rhel9.2)
2. `D_ARCH`: Compiled Architecture
3. `D_BASE_IMAGE`: Base container image
4. `D_KERNEL_VER`: The target kernel version (e.g., 5.15.0-25-generic / 5.14.0-284.32.1.el9_2.x86_64)
5. `D_OFED_VERSION`: NVIDIA NIC drivers version (e.g., 24.01-0.3.3.1)
>[!IMPORTANT]
>Check desired NVIDIA NIC drivers sources[^1] availability for designated container OS, only versions available on download page can be utilized  

## RHEL _specific_ build parameters
1. `D_BASE_IMAGE`: DriverToolKit container image
>[!IMPORTANT]
>DTK (DriverToolKit) is tightly coupled with specific kernel versions, verify match between kernel version to compile drivers for, versus DTK image.

2. `D_FINAL_BASE_IMAGE`: Final container image, to install compiled driver

## RHCOS _specific_ build parameters
1. `D_BASE_IMAGE`: DriverToolKit container image
>[!IMPORTANT]
>DTK (DriverToolKit) is tightly coupled with specific kernel version for an OpenShift release.

In order to get the specific DTK container image for a specific OpenShift release, run:
```bash
oc adm release info <OCP_VERSION> --image-for=driver-toolkit
```

For example, for OpenShift 4.16.0:
```bash
# oc adm release info 4.16.0 --image-for=driver-toolkit
quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:dde3cd6a75d865a476aa7e1cab6fa8d97742401e87e0d514f3042c3a881e301f
```

Then pull the DTK image locally using your pull-secret:
```bash
podman pull --authfile=/path/to/pull-secret.txt docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:dde3cd6a75d865a476aa7e1cab6fa8d97742401e87e0d514f3042c3a881e301f
```

2. `D_FINAL_BASE_IMAGE`: Final container image, to install compiled driver

## Examples

**RHEL example**:
```bash
# podman build \  
    --build-arg D_OS=rhel9.2 \  
    --build-arg D_ARCH=x86_64 \  
    --build-arg D_KERNEL_VER=5.14.0-284.32.1.el9_2.x86_64 \  
    --build-arg D_OFED_VERSION=24.01-0.3.3.1 \  
    --build-arg D_BASE_IMAGE="registry.redhat.io/openshift4/driver-toolkit-rhel9:v4.13.0-202309112001.p0.gd719bdc.assembly.stream" \  
    --build-arg D_FINAL_BASE_IMAGE=registry.access.redhat.com/ubi9/ubi:latest \  
    --tag 24.01-0.3.3.1-0-5.14.0-284.32.1.el9_2-rhel9.2-amd64 \  
    -f RHEL_Dockerfile \  
    --target precompiled .
```

**RHCOS example**:
```bash
# podman build \
    --build-arg D_OS=rhcos4.16 \
    --build-arg D_ARCH=x86_64 \
    --build-arg D_KERNEL_VER=5.14.0-427.22.1.el9_4.x86_64 \
    --build-arg D_OFED_VERSION=24.01-0.3.3.1 \
    --build-arg D_BASE_IMAGE="quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:dde3cd6a75d865a476aa7e1cab6fa8d97742401e87e0d514f3042c3a881e301f" \
    --build-arg D_FINAL_BASE_IMAGE=registry.access.redhat.com/ubi9/ubi:9.4 \
    --tag 24.01-0.3.3.1-0-5.14.0-427.22.1.el9_4.x86_64-rhcos4.16-amd64 \
    -f RHEL_Dockerfile \
    --target precompiled .
```

**Ubuntu example**:
```bash
# docker build \  
    --build-arg D_OS=ubuntu22.04 \  
    --build-arg D_ARCH=x86_64 \  
    --build-arg D_BASE_IMAGE=ubuntu:24.04 \  
    --build-arg D_KERNEL_VER=5.15.0-25-generic \  
    --build-arg D_OFED_VERSION=24.01-0.3.3.1 \  
    --tag 24.01-0.3.3.1-0-5.15.0-25-generic-ubuntu22.04-amd64 \  
    -f Ubuntu_Dockerfile \  
    --target precompiled .
```

>[!IMPORTANT]
>Dockerfiles contain default build parameters, which may fail build proccess on your system if not overridden.

>[!WARNING]
>Modification of D_OFED_SRC_DOWNLOAD_PATH must be tighdly coupled with corresponding update to entrypoint.sh script


[^1]: Latest NIC drivers published at [NIC drivers download center](https://network.nvidia.com/products/infiniband-drivers/linux/mlnx_ofed/), for example:  
  `https://www.mellanox.com/downloads/ofed/MLNX_OFED-24.01-0.3.3.1/MLNX_OFED_SRC-debian-24.01-0.3.3.1.tgz`

[^2]: To build RHEL based container from official repository, you need to log in to `registry.redhat.io`, run the following command:  
`podman login registry.redhat.io --username=${RH_USERNAME} --password=${RH_PASSWORD}`  
<sub> Replace RH_USERNAME and RH_PASSWORD with your Red Hat account username and password. </sub>

[^3]: Download 'oc' CLI tool from [here](https://docs.openshift.com/container-platform/latest/cli_reference/openshift_cli/getting-started-cli.html)

[^4]: Download your OpenShift pull secret from [here](https://console.redhat.com/openshift/install/pull-secret)