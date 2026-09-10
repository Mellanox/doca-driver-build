# Vendored MLNX_OFED installer scripts

DOCA 3.6 deprecated `install.pl`: the `DOCA_BASE_SRC-*` source archives are now a flat
directory of source packages and no longer ship `install.pl`, `common.pl` or
`uninstall.sh`. The driver container still builds via `install.pl` for this release, so
the scripts are vendored here and staged into the extracted source tree at image build
time by `stage_src_tree.sh`.

## Provenance

Taken from the last MLNX_OFED source archives that shipped them, `OFED-internal-26.07-0.7.7`
(DOCA 3.5):

| File | Origin archive |
| --- | --- |
| `rpm/install.pl`, `rpm/uninstall.sh` | `MLNX_OFED_SRC-26.07-0.7.7.0.tgz` |
| `deb/install.pl`, `deb/uninstall.sh` | `MLNX_OFED_SRC-debian-26.07-0.7.7.0.tgz` |
| `common.pl`, `LICENSE` | either (byte-identical in both) |

`install.pl` and `uninstall.sh` genuinely differ between the two archives, hence the
`rpm/` and `deb/` split. `rpm/` serves RHEL and SLES; `deb/` serves Ubuntu.

```
a77235ae98d3ce9671e22b68df17927db3b29bb22e2fc63963a5a4dc5d0bb77f  common.pl
a8973875e9d912631174497f8f3f15196f7cda5916ca02ef0e5db6b8c855f99e  LICENSE
0a2de055189c9854f374d9443c75ebf5fa21c5c470679f7a3334bc18c08fdc46  rpm/install.pl
b9ba633450b0c7b8d7c9981e49f242d273833db4afc24e933aaf6955033aeae5  rpm/uninstall.sh
a23752a9088778f8bb7e4976b75db4d64df86dabbeed67fd6bc9d6b1aa578470  deb/install.pl
5d280d384b027448fa84a1c5a6e9b824cc173253b49deffd55659ec52e9a3b1f  deb/uninstall.sh
```

The scripts are unmodified. They are tri-licensed under CPL 1.0, the BSD License, or
GPL v2 at the licensee's option; see the header of `install.pl` and `LICENSE`.

## Why the scripts must be copied, not symlinked

`install.pl` resolves its own location with `dirname(Cwd::abs_path $0)`, `chdir`s there,
and then derives every input and output path relative to it:

- inputs: `SRPMS/` (RPM) or `SOURCES/` (Debian)
- outputs: `RPMS/<dist>/<arch>/` or `DEBS/<distro>/<arch>/`
- `require("$WDIR/common.pl")`

`abs_path` resolves symlinks, so a symlink into this directory would make `install.pl`
look for `SRPMS/` here rather than in the staged source tree. `stage_src_tree.sh`
therefore copies the scripts into the tree root.

## ofed-scripts

DOCA 3.6 also dropped the `ofed-scripts` source package, so `--with-ofed-scripts` was
removed from the build flags. It was only ever an `ofa_req_inst`/`soft_req` in
`install.pl`, never an `ofa_req_build`, and `--build-only` resolves against
`ofa_req_build` only. Its payload (`ofed_info`, `ofed_rpm_info`, `ofed_uninstall.sh`,
`sysinfo-snapshot.py`, `profile.d` and `ld.so.conf.d` fragments) is unused by this
repository. `/usr/share/mlnx_ofed/mod_load_funcs` comes from `mlnx-ofa_kernel`, not from
`ofed-scripts`.
