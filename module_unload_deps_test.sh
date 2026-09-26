#!/usr/bin/env bash

set -euo pipefail

# Argument parsing: accepts (in any order)
#   [<script.sh>]   path to module_unload_deps.sh  (must end in .sh)
#   [<lsmod_file>]  external lsmod snapshot to test (anything else)
# Examples:
#   bash module_unload_deps_test.sh
#   bash module_unload_deps_test.sh lsmod_t1
#   bash module_unload_deps_test.sh ./module_unload_deps.sh lsmod_t1
SCRIPT_PATH=""
EXTRA_LSMOD=""
for _arg in "$@"; do
    case "$_arg" in
        *.sh) SCRIPT_PATH="$_arg" ;;
        *)    EXTRA_LSMOD="$_arg" ;;
    esac
done
SCRIPT_PATH="${SCRIPT_PATH:-./module_unload_deps.sh}"

if [[ "${SCRIPT_PATH:0:1}" != "/" ]]; then
    SCRIPT_PATH="$(realpath "$SCRIPT_PATH")"
fi

if [[ ! -f "$SCRIPT_PATH" ]]; then
    echo "Script not found: $SCRIPT_PATH" >&2
    exit 1
fi

if [[ -n "$EXTRA_LSMOD" && ! -f "$EXTRA_LSMOD" ]]; then
    echo "External lsmod file not found: $EXTRA_LSMOD" >&2
    exit 1
fi

STEP=0
step() {
    STEP=$((STEP + 1))
    echo "[$STEP] $*"
}

show_file() {
    local file="$1"
    if [[ -s "$file" ]]; then
        awk '1' "$file"
    else
        echo "(empty)"
    fi
}

assert_contains() {
    local needle="$1"
    local file="$2"
    step "Assert '$needle' exists in $file"
    if ! awk -v n="$needle" 'index($0, n) { found=1; exit } END { exit(found ? 0 : 1) }' "$file"; then
        echo "ASSERT FAILED: expected to find '$needle' in $file" >&2
        echo "---- $file ----" >&2
        awk '1' "$file" >&2
        return 1
    fi
}

assert_not_contains() {
    local needle="$1"
    local file="$2"
    step "Assert '$needle' does not exist in $file"
    if awk -v n="$needle" 'index($0, n) { found=1; exit } END { exit(found ? 0 : 1) }' "$file"; then
        echo "ASSERT FAILED: expected to NOT find '$needle' in $file" >&2
        echo "---- $file ----" >&2
        awk '1' "$file" >&2
        return 1
    fi
}

# Assert that $first appears on an earlier line than $second in $file.
assert_before() {
    local first="$1"
    local second="$2"
    local file="$3"
    step "Assert '$first' appears before '$second' in $file"
    local line_first line_second
    line_first=$(awk -v n="$first" 'index($0, n) { print NR; exit }' "$file")
    line_second=$(awk -v n="$second" 'index($0, n) { print NR; exit }' "$file")
    if [[ -z "$line_first" || -z "$line_second" ]]; then
        echo "ASSERT FAILED: '$first' (line ${line_first:-missing}) or '$second' (line ${line_second:-missing}) not found in $file" >&2
        awk '1' "$file" >&2
        return 1
    fi
    if [[ "$line_first" -ge "$line_second" ]]; then
        echo "ASSERT FAILED: '$first' (line $line_first) should appear before '$second' (line $line_second) in $file" >&2
        awk '1' "$file" >&2
        return 1
    fi
}


# Run modules_to_unload against a real lsmod snapshot.
# Strategy (no dependency analysis in the test — that is module_unload_deps.sh's job):
#   1. Find the first well-known NVIDIA/RDMA hub module present in the file
#      (plain name lookup, not graph traversal).
#   2. Run the script once with that root module to obtain its dependency tree.
#      rc=0: clean tree.  rc=1: user-count mismatch(es) in the tree — the
#      script is working correctly, the stack just cannot be fully unloaded.
#      rc=2: parser/logic error — hard failure.
#   3. Run the script individually for every module in that tree.
#      rc=0 → pass, rc=1 → warning (valid mismatch), rc=2 → failure.
run_external_lsmod_tests() {
    local lsmod_file="$1"
    local abs_file
    abs_file="$(realpath "$lsmod_file")"

    echo
    echo "── External lsmod: $(basename "$abs_file") ─────────────────────────────────────"

    local ext_dir="$TMP_ROOT/ext_lsmod"
    mkdir -p "$ext_dir"
    cp "$abs_file" "$ext_dir/lsmod_output"

    # Plain name lookup: find the first well-known NVIDIA/RDMA hub module in the file.
    local root_mod=""
    for candidate in ib_core mlx5_core mlx_compat rdma_cm mlx5_ib ib_uverbs; do
        if awk -v m="$candidate" \
               '/^Module[[:space:]]/{next} NF>=1 && $1==m {found=1} END{exit !found}' \
               "$abs_file"; then
            root_mod="$candidate"
            break
        fi
    done

    if [ -z "$root_mod" ]; then
        echo "  [SKIP] no well-known NVIDIA/RDMA module found in file"
        return 0
    fi

    # Step 1: let the script resolve the full dependency tree for the root module.
    # rc=1 is a valid outcome (user-count mismatch) — the tree is still emitted on
    # stdout, so step 2 can proceed.  Only rc=2 is a hard parser/logic failure.
    local rc=0 tree total
    tree=$(cd "$ext_dir" && bash "$SCRIPT_PATH" "$root_mod" 2>"$ext_dir/err.txt") || rc=$?
    case $rc in
        0) ;;
        1) printf '  [WARN] root %s  rc=1  (user-count mismatch — stack cannot be fully unloaded)\n' "$root_mod"
           printf '         %s\n' "$(cat "$ext_dir/err.txt")" ;;
        2) printf '  [FAIL] %s  rc=2  (root resolution failed: module not found)\n' "$root_mod"
           cat "$ext_dir/err.txt"
           return 1 ;;
        *) printf '  [FAIL] %s  rc=%d  (unexpected)\n' "$root_mod" "$rc"
           cat "$ext_dir/err.txt"
           return 1 ;;
    esac
    if [ -z "$tree" ]; then
        printf '  [FAIL] %s  (empty dependency tree output)\n' "$root_mod"
        return 1
    fi
    total=$(printf '%s\n' "$tree" | grep -c . 2>/dev/null || echo 0)
    printf 'Root: %s  →  %d modules in dependency tree\n' "$root_mod" "$total"

    # Step 2: run the script for every module that belongs to the resolved tree.
    # Only these modules are tested; everything else in the lsmod file is ignored.
    local passed=0 warned=0 failed=0
    for mod in $tree; do
        local mod_rc=0 mod_out count
        mod_out=$(cd "$ext_dir" && bash "$SCRIPT_PATH" "$mod" 2>"$ext_dir/err.txt") || mod_rc=$?
        count=$(printf '%s\n' "$mod_out" | grep -c . 2>/dev/null || echo 0)
        case $mod_rc in
            0) printf '  [PASS] %-24s rc=0  (%d modules in unload order)\n' "$mod" "$count"
               passed=$((passed + 1)) ;;
            1) printf '  [WARN] %-24s rc=1  (unexpected users within tree)\n' "$mod"
               printf '         stderr: %s\n' "$(cat "$ext_dir/err.txt")"
               warned=$((warned + 1)) ;;
            2) printf '  [FAIL] %-24s rc=2  (parser failed to find tree module)\n' "$mod"
               failed=$((failed + 1)) ;;
            *) printf '  [FAIL] %-24s rc=%d  (unexpected)\n' "$mod" "$mod_rc"
               failed=$((failed + 1)) ;;
        esac
    done

    echo
    printf 'Result: %d passed, %d warning(s), %d failed  [root: %s, file: %s]\n' \
        "$passed" "$warned" "$failed" "$root_mod" "$(basename "$abs_file")"

    # rc=1 (user-count mismatch) means modules cannot be unloaded — treat as failure.
    [ "$failed" -eq 0 ] && [ "$warned" -eq 0 ]
}

run_case() {
    local case_name="$1"
    local target="$2"
    local expected_rc="$3"
    local content="$4"

    local case_dir="$TMP_ROOT/$case_name"
    step "Prepare case '$case_name' in $case_dir"
    mkdir -p "$case_dir"
    printf '%s\n' "$content" >"$case_dir/lsmod_output"
    step "Run target '$target' for case '$case_name' (expected rc=$expected_rc)"

    set +e
    (
        cd "$case_dir" || exit 99
        bash "$SCRIPT_PATH" "$target" >stdout.txt 2>stderr.txt
    )
    local rc=$?
    set -e

    step "Case '$case_name' finished with rc=$rc"
    echo "----- $case_name stdout -----"
    show_file "$case_dir/stdout.txt"
    echo "----- $case_name stderr -----"
    show_file "$case_dir/stderr.txt"

    if [[ "$rc" -ne "$expected_rc" ]]; then
        echo "ASSERT FAILED: $case_name expected rc=$expected_rc got rc=$rc" >&2
        awk '1' "$case_dir/stdout.txt" >&2
        awk '1' "$case_dir/stderr.txt" >&2
        return 1
    fi
}

# Like run_case but invokes --cmd (modules_unload_command) with inventory env vars.
# inv_base:        NVIDIA_NIC_DRIVERS_INVENTORY_PATH value (pass "" to leave unset)
# fake_mods:       space-separated module names fed to the dpkg-deb/rpm shims via
#                  FAKE_INVENTORY_MODULES; the shims return these as .ko paths
#                  without touching any real file — pass "" to report an empty
#                  package (all modules missing)
# fake_compressed: "1" → shims emit .ko.xz paths; default "0" → plain .ko
# KVER and DRVVER are fixed at 5.15.0-test / 25.04
run_cmd_case() {
    local case_name="$1"
    local target="$2"
    local expected_rc="$3"
    local inv_base="$4"
    local content="$5"
    local fake_mods="${6:-}"
    local fake_compressed="${7:-0}"

    local kver="5.15.0-test"
    local drvver="25.04"
    local case_dir="$TMP_ROOT/$case_name"
    step "Prepare cmd case '$case_name' in $case_dir"
    mkdir -p "$case_dir"
    printf '%s\n' "$content" >"$case_dir/lsmod_output"
    step "Run --cmd '$target' for case '$case_name' (expected rc=$expected_rc)"

    set +e
    (
        cd "$case_dir" || exit 99
        if [[ -n "$inv_base" ]]; then
            NVIDIA_NIC_DRIVERS_INVENTORY_PATH="$inv_base" \
            FULL_KVER="$kver" \
            NVIDIA_NIC_DRIVER_VER="$drvver" \
            FAKE_INVENTORY_MODULES="$fake_mods" \
            FAKE_COMPRESSED="$fake_compressed" \
            FAKE_KVER="$kver" \
            bash "$SCRIPT_PATH" --cmd "$target" >stdout.txt 2>stderr.txt
        else
            env -u NVIDIA_NIC_DRIVERS_INVENTORY_PATH \
                -u FULL_KVER \
                -u NVIDIA_NIC_DRIVER_VER \
                -u FAKE_INVENTORY_MODULES \
                -u FAKE_COMPRESSED \
                bash "$SCRIPT_PATH" --cmd "$target" >stdout.txt 2>stderr.txt
        fi
    )
    local rc=$?
    set -e

    step "Case '$case_name' finished with rc=$rc"
    echo "----- $case_name stdout -----"
    show_file "$case_dir/stdout.txt"
    echo "----- $case_name stderr -----"
    show_file "$case_dir/stderr.txt"

    if [[ "$rc" -ne "$expected_rc" ]]; then
        echo "ASSERT FAILED: $case_name expected rc=$expected_rc got rc=$rc" >&2
        awk '1' "$case_dir/stdout.txt" >&2
        awk '1' "$case_dir/stderr.txt" >&2
        return 1
    fi
}

TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT
step "Using temporary test directory: $TMP_ROOT"

# Install virtual package shims so inventory tests are fully self-contained:
# no dpkg-deb, rpm, ar, tar, or compiled driver packages required.
#
# All module data is supplied via env vars set by run_cmd_case:
#   FAKE_INVENTORY_MODULES  space-separated module names the "package" contains
#   FAKE_COMPRESSED         "1" → emit .ko.xz paths; "0" → plain .ko (default)
#   FAKE_KVER               kernel version string used in the emitted path
#
# dpkg-deb --fsys-tarfile <pkg>  →  emits one ./lib/modules/.../mod.ko per line
# tar -t                          →  passes stdin straight through (already a listing)
# rpm -qlp <pkg>                  →  same as dpkg-deb
mkdir -p "$TMP_ROOT/bin"

cat > "$TMP_ROOT/bin/dpkg-deb" << 'EOF'
#!/bin/sh
case "$1" in
    --fsys-tarfile)
        sfx=".ko"
        [ "${FAKE_COMPRESSED:-0}" = "1" ] && sfx=".ko.xz"
        for mod in ${FAKE_INVENTORY_MODULES:-}; do
            printf './lib/modules/%s/updates/%s%s\n' \
                "${FAKE_KVER:-5.15.0}" "$mod" "$sfx"
        done
        ;;
    *) echo "fake dpkg-deb: unsupported: $*" >&2; exit 1 ;;
esac
EOF

cat > "$TMP_ROOT/bin/tar" << 'EOF'
#!/bin/sh
case "$1" in
    -t) cat ;;
    *) echo "fake tar: unsupported: $*" >&2; exit 1 ;;
esac
EOF

cat > "$TMP_ROOT/bin/rpm" << 'EOF'
#!/bin/sh
case "$1" in
    -qlp)
        sfx=".ko"
        [ "${FAKE_COMPRESSED:-0}" = "1" ] && sfx=".ko.xz"
        for mod in ${FAKE_INVENTORY_MODULES:-}; do
            printf '/lib/modules/%s/updates/%s%s\n' \
                "${FAKE_KVER:-5.15.0}" "$mod" "$sfx"
        done
        ;;
    *) echo "fake rpm: unsupported: $*" >&2; exit 1 ;;
esac
EOF

chmod +x "$TMP_ROOT/bin/dpkg-deb" "$TMP_ROOT/bin/tar" "$TMP_ROOT/bin/rpm"
export PATH="$TMP_ROOT/bin:$PATH"
step "Installed virtual package shims (dpkg-deb, tar, rpm) in $TMP_ROOT/bin"

COMMON_HEADER="Module                  Size  Used by"

# ── Existing cases ─────────────────────────────────────────────────────────────

CASE_COMPLEX_OK="$COMMON_HEADER
core                    1000  3 netA,storageA,virtA
netA                     900  2 app1,app2
storageA                 850  1 app2
virtA                    700  1 app3
app1                     600  0
app2                     550  1 monitor
app3                     500  0
monitor                  450  0
compat                   400  1 core
fw                       300  1 compat"

CASE_MISMATCH_IN_PATH="$COMMON_HEADER
core                    1000  3 netA,storageA,virtA
netA                     900  2 app1,app2
storageA                 850  1 app2
virtA                    700  1 app3
app1                     600  0
app2                     550  1 monitor
app3                     500  0
monitor                  450  0
compat                   400  2 core
fw                       300  1 compat"

CASE_UNRELATED_MISMATCH="$COMMON_HEADER
core                    1000  3 netA,storageA,virtA
netA                     900  2 app1,app2
storageA                 850  1 app2
virtA                    700  1 app3
app1                     600  0
app2                     550  1 monitor
app3                     500  0
monitor                  450  0
compat                   400  1 core
fw                       300  1 compat
orphanA                  200  4 orphanB,orphanC
orphanB                  150  0
orphanC                  120  0"

CASE_BAD_HEADER="Modulezzzzz                  Size  Used by
core                    1000  0"

run_case "complex_ok" "core" 0 "$CASE_COMPLEX_OK"
assert_contains "core" "$TMP_ROOT/complex_ok/stdout.txt"
assert_contains "netA" "$TMP_ROOT/complex_ok/stdout.txt"
assert_contains "storageA" "$TMP_ROOT/complex_ok/stdout.txt"
assert_contains "virtA" "$TMP_ROOT/complex_ok/stdout.txt"
assert_contains "compat" "$TMP_ROOT/complex_ok/stdout.txt"
assert_contains "fw" "$TMP_ROOT/complex_ok/stdout.txt"
assert_not_contains "Unexpected number of users for module" "$TMP_ROOT/complex_ok/stderr.txt"

run_case "mismatch_in_path" "core" 1 "$CASE_MISMATCH_IN_PATH"
assert_contains "Unexpected number of users for module 'compat'" "$TMP_ROOT/mismatch_in_path/stderr.txt"
assert_contains "module 'core' cannot be unloaded due to unexpected users in module 'compat'" "$TMP_ROOT/mismatch_in_path/stderr.txt"

run_case "unrelated_mismatch" "core" 0 "$CASE_UNRELATED_MISMATCH"
assert_not_contains "Unexpected number of users for module 'orphanA'" "$TMP_ROOT/unrelated_mismatch/stderr.txt"
assert_not_contains "cannot be unloaded" "$TMP_ROOT/unrelated_mismatch/stderr.txt"

run_case "module_not_loaded" "missing_module" 2 "$CASE_COMPLEX_OK"
assert_contains "module 'missing_module' is not loaded" "$TMP_ROOT/module_not_loaded/stderr.txt"

run_case "bad_header" "core" 1 "$CASE_BAD_HEADER"
assert_contains "invalid lsmod header line" "$TMP_ROOT/bad_header/stderr.txt"

# ── Group 1: graph corner cases ────────────────────────────────────────────────

# Single isolated module: no users, no deps. Must unload just itself.
CASE_SINGLE_MODULE="$COMMON_HEADER
standalone               500  0"

run_case "single_module" "standalone" 0 "$CASE_SINGLE_MODULE"
assert_contains "standalone" "$TMP_ROOT/single_module/stdout.txt"
assert_not_contains "Unexpected number" "$TMP_ROOT/single_module/stderr.txt"

# Valid lsmod with no header line; parser must not reject it.
CASE_NO_HEADER="core                    1000  2 netA,storageA
netA                     900  0
storageA                 850  0"

run_case "no_header" "core" 0 "$CASE_NO_HEADER"
assert_contains "core" "$TMP_ROOT/no_header/stdout.txt"
assert_contains "netA" "$TMP_ROOT/no_header/stdout.txt"
assert_contains "storageA" "$TMP_ROOT/no_header/stdout.txt"

# Linear chain: top depends on mid, mid depends on bot.
# Unload order must be: top, mid, bot (dependents first).
CASE_CHAIN_LINEAR="$COMMON_HEADER
bot                      600  1 mid
mid                      800  1 top
top                     1000  0"

run_case "chain_linear" "bot" 0 "$CASE_CHAIN_LINEAR"
assert_contains "top" "$TMP_ROOT/chain_linear/stdout.txt"
assert_contains "mid" "$TMP_ROOT/chain_linear/stdout.txt"
assert_contains "bot" "$TMP_ROOT/chain_linear/stdout.txt"
assert_before "top" "mid" "$TMP_ROOT/chain_linear/stdout.txt"
assert_before "mid" "bot" "$TMP_ROOT/chain_linear/stdout.txt"

# Diamond: app1 and app2 both depend on lib.
# Both apps must appear in output before lib.
CASE_DIAMOND="$COMMON_HEADER
lib                     1000  2 app1,app2
app1                     800  0
app2                     700  0"

run_case "diamond_dependency" "lib" 0 "$CASE_DIAMOND"
assert_contains "lib" "$TMP_ROOT/diamond_dependency/stdout.txt"
assert_contains "app1" "$TMP_ROOT/diamond_dependency/stdout.txt"
assert_contains "app2" "$TMP_ROOT/diamond_dependency/stdout.txt"
assert_before "app1" "lib" "$TMP_ROOT/diamond_dependency/stdout.txt"
assert_before "app2" "lib" "$TMP_ROOT/diamond_dependency/stdout.txt"

# Two modules with mismatched user counts in the connected path.
# DFS order: leaf, middle, core — so leaf is the first mismatch reported.
CASE_MULTIPLE_MISMATCHES="$COMMON_HEADER
core                    1000  1 middle
middle                   800  4 leaf
leaf                     600  2"

run_case "multiple_mismatches" "core" 1 "$CASE_MULTIPLE_MISMATCHES"
assert_contains "Unexpected number of users for module 'middle'" "$TMP_ROOT/multiple_mismatches/stderr.txt"
assert_contains "Unexpected number of users for module 'leaf'" "$TMP_ROOT/multiple_mismatches/stderr.txt"
assert_contains "module 'core' cannot be unloaded due to unexpected users in module 'leaf'" "$TMP_ROOT/multiple_mismatches/stderr.txt"

# ── Group 2: inventory (--cmd) corner cases ────────────────────────────────────
#
# All inventory tests share one virtual directory.  Each test populates the
# "package contents" purely via FAKE_INVENTORY_MODULES — no compiled drivers,
# no real .deb/.rpm files, no ar/tar required.  The stub .deb files are empty
# placeholders that make the *.deb glob in _check_modules_in_inventory match;
# the dpkg-deb shim ignores their contents entirely.

SIMPLE_LSMOD="$COMMON_HEADER
base                    1000  1 user
user                     800  0"

KVER_TEST="5.15.0-test"
DRVVER_TEST="25.04"

# Shared virtual inventory: one directory, two empty stub packages.
# A second stub (virtual2.deb) is present so cmd_inventory_multi_pkg exercises
# the multi-package for-loop without needing any extra file content.
VIRTUAL_INV="$TMP_ROOT/inventory"
VIRTUAL_INV_PKG="${VIRTUAL_INV}/${KVER_TEST}/${DRVVER_TEST}"
mkdir -p "$VIRTUAL_INV_PKG"
touch "$VIRTUAL_INV_PKG/virtual.deb" "$VIRTUAL_INV_PKG/virtual2.deb"
step "Virtual inventory: $VIRTUAL_INV_PKG (stubs: virtual.deb, virtual2.deb)"

# No inventory env vars set: check skipped, modprobe command produced.
run_cmd_case "cmd_no_inventory_env" "base" 0 "" "$SIMPLE_LSMOD"
assert_contains "modprobe -r" "$TMP_ROOT/cmd_no_inventory_env/stdout.txt"

# Inventory env vars point to a non-existent directory: check skipped.
run_cmd_case "cmd_inventory_dir_missing" "base" 0 "/nonexistent/path" "$SIMPLE_LSMOD"
assert_contains "modprobe -r" "$TMP_ROOT/cmd_inventory_dir_missing/stdout.txt"

# Virtual inventory reports both modules → all covered, command produced.
run_cmd_case "cmd_inventory_all_covered" "base" 0 "$VIRTUAL_INV" "$SIMPLE_LSMOD" "user base"
assert_contains "modprobe -r" "$TMP_ROOT/cmd_inventory_all_covered/stdout.txt"
assert_contains "user"        "$TMP_ROOT/cmd_inventory_all_covered/stdout.txt"
assert_contains "base"        "$TMP_ROOT/cmd_inventory_all_covered/stdout.txt"

# Virtual inventory reports only 'base' → 'user' is missing, rc=1.
run_cmd_case "cmd_inventory_one_missing" "base" 1 "$VIRTUAL_INV" "$SIMPLE_LSMOD" "base"
assert_contains "user" "$TMP_ROOT/cmd_inventory_one_missing/stderr.txt"

# Virtual inventory reports .ko.xz paths → sed suffix-stripping must handle compression.
run_cmd_case "cmd_inventory_compressed" "base" 0 "$VIRTUAL_INV" "$SIMPLE_LSMOD" "user base" "1"
assert_contains "modprobe -r" "$TMP_ROOT/cmd_inventory_compressed/stdout.txt"

# Two stub packages present; shim is called once per stub.
# Both calls return the same FAKE_INVENTORY_MODULES, exercising the aggregation loop.
run_cmd_case "cmd_inventory_multi_pkg" "base" 0 "$VIRTUAL_INV" "$SIMPLE_LSMOD" "user base"
assert_contains "modprobe -r" "$TMP_ROOT/cmd_inventory_multi_pkg/stdout.txt"
assert_contains "user"        "$TMP_ROOT/cmd_inventory_multi_pkg/stdout.txt"
assert_contains "base"        "$TMP_ROOT/cmd_inventory_multi_pkg/stdout.txt"

echo "All module_unload_deps test cases passed."

if [[ -n "$EXTRA_LSMOD" ]]; then
    run_external_lsmod_tests "$EXTRA_LSMOD"
fi
