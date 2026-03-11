#!/bin/sh
# Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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

_LOG_FILE=/tmp/module_unload_deps.log

# Append a timestamped line to the log file.
_log() {
    printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >> "$_LOG_FILE"
}

# awk program for modules_to_unload.
# Variables: targets (space-sep list), log_file, ts, lsmod_src
# Input:     lsmod output (with or without header line)
# Stdout:    module names in safe-unload order (dependents first, providers last)
# Stderr:    diagnostic messages
# Exit:      0 ok  |  1 cannot unload (unexpected users)  |  2 module not loaded
#
# Algorithm:
#   1. Parse lsmod into per-module user counts and user-name lists.
#      Also build the inverse map: for each module U listed as a user of P,
#      record P as a provider of U.  This gives us both graph directions.
#   2. Flood-fill (bidirectional): seed the unload set with the targets, then
#      repeatedly add (a) modules that use something in the set and (b) modules
#      that something in the set uses (providers), until stable.
#      This is the same connected-component discovery as BFS but written as a
#      simple "scan until nothing changes" loop -- O(n^2) but n is tiny.
#   3. Leaf-peel: emit a module once all of its in-set users are emitted,
#      repeating until the set is empty.  Equivalent to DFS post-order
#      topological sort but written as a plain O(n^2) loop.
#   4. Validate: every module in the set must have no more users than are
#      listed in lsmod (extra unlisted users are outside our set, making
#      unload unsafe).
#
# Single quotes inside this awk program are written as sprintf("%c",39) via
# the q variable so the program can be stored in a shell single-quoted string.
_LSMOD_DEPS_AWK='
BEGIN {
    q = sprintf("%c", 39)
    ntargets = split(targets, target_arr, " ")
    if (log_file != "") {
        print "---" >> log_file
        print "[" ts "] [modules_to_unload] targets=" targets " source=" lsmod_src >> log_file
        print "[" ts "] [parse] begin" >> log_file
    }
}

/^[[:space:]]*$/ { next }

/^Module/ {
    if ($0 ~ /^Module[[:space:]]+Size[[:space:]]+Used[[:space:]]+by$/) {
        if (log_file != "") print "[" ts "] [parse] lsmod header skipped" >> log_file
        next
    }
    if (log_file != "") print "[" ts "] [parse] ERROR invalid header: " $0 >> log_file
    print "invalid lsmod header line: " q $0 q > "/dev/stderr"
    parse_error = 1; exit 1
}

{
    mod = $1
    if (mod == "") next
    if ($3 !~ /^[0-9]+$/) {
        if (log_file != "") print "[" ts "] [parse] ERROR invalid used-by count for " mod ": " $3 >> log_file
        print "invalid used-by count for module " q mod q ": " q $3 q > "/dev/stderr"
        parse_error = 1; exit 1
    }
    count[mod] = $3 + 0
    n = split($4, a, ",")
    listed[mod] = n
    for (i = 1; i <= n; i++) {
        u = a[i]
        if (u != "") {
            users_of[mod, i] = u                           # upward edges: u uses mod
            provides_for[u, ++provides_count[u]] = mod     # downward edges: mod is a provider of u
        }
    }
    if (log_file != "") print "[" ts "] [parse] " mod ": count=" $3 " users=" $4 >> log_file
    parsed_count++
}

END {
    if (parse_error) exit 1

    if (log_file != "") print "[" ts "] [parse] done: " parsed_count " modules parsed" >> log_file

    # Step 1: verify all targets are loaded.
    for (ti = 1; ti <= ntargets; ti++) {
        t = target_arr[ti]
        if (!(t in count)) {
            if (log_file != "") print "[" ts "] [existence] " t " is NOT loaded" >> log_file
            print "module " q t q " is not loaded" > "/dev/stderr"
            exit 2
        }
        if (log_file != "") print "[" ts "] [existence] " t " is loaded (count=" count[t] ")" >> log_file
    }

    # Step 2: flood-fill -- expand the unload set in both graph directions.
    for (ti = 1; ti <= ntargets; ti++)
        in_set[target_arr[ti]] = 1

    changed = 1
    while (changed) {
        changed = 0
        for (mod in in_set) {
            # Upward: add modules that use mod.
            for (i = 1; i <= listed[mod]; i++) {
                u = users_of[mod, i]
                if (!(u in in_set)) { in_set[u] = 1; changed = 1 }
            }
            # Downward: add modules that mod depends on (providers).
            for (i = 1; i <= provides_count[mod]; i++) {
                p = provides_for[mod, i]
                if (!(p in in_set)) { in_set[p] = 1; changed = 1 }
            }
        }
    }

    if (log_file != "") {
        set_list = ""
        for (mod in in_set) set_list = set_list " " mod
        print "[" ts "] [flood-fill] unload set:" set_list >> log_file
    }

    # Step 3: leaf-peel -- emit a module once all its in-set users are emitted.
    # Print each module as it is emitted; validate counts in the same pass.
    remaining = length(in_set)
    first_mismatch = ""
    mismatch_count = 0
    order_list = ""
    while (remaining > 0) {
        progress = 0
        for (mod in in_set) {
            if (emitted[mod]) continue
            ready = 1
            for (i = 1; i <= listed[mod]; i++) {
                u = users_of[mod, i]
                if ((u in in_set) && !emitted[u]) { ready = 0; break }
            }
            if (!ready) continue
            print mod
            emitted[mod] = 1
            remaining--
            progress = 1
            if (log_file != "") order_list = order_list " " mod
            # Step 4: validate -- no module may have more users than listed in lsmod.
            if (count[mod] != listed[mod]) {
                if (log_file != "") print "[" ts "] [validate] MISMATCH " mod ": reported=" count[mod] " listed=" listed[mod] >> log_file
                print "Unexpected number of users for module " q mod q ": reported " count[mod] ", listed " listed[mod] > "/dev/stderr"
                if (first_mismatch == "") first_mismatch = mod
                mismatch_count++
            } else {
                if (log_file != "") print "[" ts "] [validate] " mod ": count=" count[mod] " OK" >> log_file
            }
        }
        if (!progress) {
            print "circular dependency in module graph" > "/dev/stderr"
            exit 1
        }
    }

    if (log_file != "") {
        print "[" ts "] [leaf-peel] unload order:" order_list >> log_file
        if (mismatch_count == 0)
            print "[" ts "] [validate] all " length(in_set) " user counts match" >> log_file
        else
            print "[" ts "] [validate] " mismatch_count " mismatch(es), first=" first_mismatch >> log_file
    }

    if (first_mismatch != "") {
        if (log_file != "") print "[" ts "] [modules_to_unload] FAILED: unexpected users in " first_mismatch >> log_file
        print (ntargets > 1 ? "modules" : "module") " " q targets q " cannot be unloaded due to unexpected users in module " q first_mismatch q > "/dev/stderr"
        exit 1
    }

    if (log_file != "") print "[" ts "] [modules_to_unload] done: rc=0" >> log_file
}
'
# Print modules that must be unloaded for one or more target modules.
# Output is the full bidirectional connected component: modules that depend on
# the targets (upward) AND modules the targets depend on (downward providers).
# Output order is unload-safe: dependents first, providers later.

modules_to_unload() {
    if [ $# -lt 1 ]; then
        echo "usage: modules_to_unload <module_name> [<module_name>...]" >&2
        return 1
    fi
    local rc=0
    local ts
    ts=$(date '+%Y-%m-%d %H:%M:%S')
    if [ -f "lsmod_output" ]; then
        awk -v targets="$*" -v log_file="$_LOG_FILE" -v ts="$ts" \
            -v lsmod_src="file:lsmod_output" "$_LSMOD_DEPS_AWK" lsmod_output || rc=$?
    else
        lsmod | awk -v targets="$*" -v log_file="$_LOG_FILE" -v ts="$ts" \
                    -v lsmod_src="live" "$_LSMOD_DEPS_AWK" || rc=$?
    fi
    [ "$rc" -ne 0 ] && _log "[modules_to_unload] FAILED: rc=$rc"
    return $rc
}

# Verify that every module in the space-separated list has a compiled .ko file
# in the driver package inventory.
#
# Inventory path is derived from environment variables set by the caller:
#   NVIDIA_NIC_DRIVERS_INVENTORY_PATH / FULL_KVER / NVIDIA_NIC_DRIVER_VER
#
# Package format is auto-detected (dpkg-deb for Debian/Ubuntu, rpm for RPM distros).
# Returns 0 if all modules are covered or if no inventory path is available.
# Returns 1 and prints missing modules to stderr if any are absent.
_check_modules_in_inventory() {
    local modules_list="$1"

    if [ -z "${NVIDIA_NIC_DRIVERS_INVENTORY_PATH:-}" ] || \
       [ -z "${FULL_KVER:-}" ] || \
       [ -z "${NVIDIA_NIC_DRIVER_VER:-}" ]; then
        _log "[inventory] env vars not set, skipping package coverage check"
        return 0
    fi

    local inv_path="${NVIDIA_NIC_DRIVERS_INVENTORY_PATH}/${FULL_KVER}/${NVIDIA_NIC_DRIVER_VER}"

    if [ ! -d "$inv_path" ]; then
        _log "[inventory] path '$inv_path' not found, skipping package coverage check"
        return 0
    fi

    _log "[inventory] checking path: $inv_path"

    # Stream .ko basenames from packages without extracting to disk.
    # Handles compressed modules: .ko.xz, .ko.gz, .ko.zst
    local pkg_ext pkg_list_cmd
    if command -v dpkg-deb > /dev/null 2>&1; then
        pkg_ext="deb"
        pkg_list_cmd='dpkg-deb --fsys-tarfile "$pkg" | tar -t'
    elif command -v rpm > /dev/null 2>&1; then
        pkg_ext="rpm"
        pkg_list_cmd='rpm -qlp "$pkg" 2>/dev/null'
    else
        _log "[inventory] no dpkg-deb or rpm available, skipping package coverage check"
        return 0
    fi

    local available_kos=""
    local pkg pkg_count=0 ko_count total_ko_count=0
    for pkg in "${inv_path}"/*."$pkg_ext"; do
        [ -f "$pkg" ] || continue
        local pkg_kos
        pkg_kos=$(eval "$pkg_list_cmd" | grep '\.ko' | sed 's|.*/||; s|\.ko.*||')
        ko_count=$(printf '%s\n' "$pkg_kos" | grep -c . 2>/dev/null || echo 0)
        available_kos="${available_kos}${pkg_kos}
"
        total_ko_count=$((total_ko_count + ko_count))
        pkg_count=$((pkg_count + 1))
        _log "[inventory] scanned $(basename "$pkg"): $ko_count kernel modules"
    done
    _log "[inventory] scanned $pkg_count .${pkg_ext} package(s), $total_ko_count .ko files total"

    local missing=""
    local found_count=0
    for mod in $modules_list; do
        if printf '%s\n' "$available_kos" | grep -qx "$mod"; then
            _log "[inventory] $mod: found in packages"
            found_count=$((found_count + 1))
        else
            _log "[inventory] $mod: NOT found in packages"
            missing="$missing $mod"
        fi
    done

    if [ -n "$missing" ]; then
        _log "[inventory] FAILED: missing modules:$missing"
        echo "modules scheduled for unload have no compiled package in ${inv_path}:${missing}" >&2
        return 1
    fi

    _log "[inventory] all $found_count module(s) covered by packages"
}

modules_unload_command() {
    if [ $# -lt 1 ]; then
        echo "usage: modules_unload_command <module_name> [<module_name>...]" >&2
        return 1
    fi
    local root_modules="$*"
    _log "[modules_unload_command] start: targets=$root_modules"

    local output rc
    output=$(modules_to_unload "$@") || {
        rc=$?
        _log "[modules_unload_command] FAILED: modules_to_unload rc=$rc"
        _log "[SUMMARY] targets=$root_modules: modules CANNOT be unloaded (rc=$rc)"
        return $rc
    }

    # word-split newline-separated names into positional params, then join with spaces
    # shellcheck disable=SC2086
    set -- $output

    # Ensure every root module is present in the unload list.
    for _t in $root_modules; do
        case " $* " in
            *" $_t "*) ;;
            *) set -- "$@" "$_t" ;;
        esac
    done

    _check_modules_in_inventory "$*" || {
        _log "[modules_unload_command] FAILED: inventory check"
        _log "[SUMMARY] targets=$root_modules: modules CANNOT be unloaded (rc=1)"
        return 1
    }

    local cmd="modprobe -r $*"
    _log "[modules_unload_command] done: $cmd"
    _log "[SUMMARY] targets=$root_modules: all modules can be unloaded (rc=0)"
    echo "$cmd"
}

case "$(basename -- "$0")" in
    module_unload_deps.sh)
        set -eu
        if [ $# -ge 2 ] && [ "$1" = "--cmd" ]; then
            shift
            modules_unload_command "$@"
        else
            modules_to_unload "$@"
        fi
        ;;
esac
