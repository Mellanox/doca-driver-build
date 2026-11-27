#!/bin/bash
# Copyright (c) 2024 NVIDIA CORPORATION & AFFILIATES. All rights reserved.

# Thin launcher for the OpenShift Driver Toolkit (DTK) sidecar.
# The driver container publishes this script, the entrypoint binary and dtk.env into the
# shared DTK directory; the sidecar executes this script, which hands over to the Go
# implementation (`entrypoint dtk-build`).

set -e

script_dir="$(dirname "$0")"

# Load environment variables if file exists
if [ -f "${script_dir}/dtk.env" ]; then
    source "${script_dir}/dtk.env"
fi

exec "${script_dir}/entrypoint" dtk-build
