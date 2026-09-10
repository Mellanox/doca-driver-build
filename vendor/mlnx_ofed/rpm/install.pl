#!/usr/bin/perl
# ex:ts=4:sw=4:sts=4:et
# -*- tab-width: 4; c-basic-offset: 4; indent-tabs-mode: nil -*-
#
# Copyright (c) 2012 Mellanox Technologies. All rights reserved.
#
# This Software is licensed under one of the following licenses:
#
# 1) under the terms of the "Common Public License 1.0" a copy of which is
#    available from the Open Source Initiative, see
#    http://www.opensource.org/licenses/cpl.php.
#
# 2) under the terms of the "The BSD License" a copy of which is
#    available from the Open Source Initiative, see
#    http://www.opensource.org/licenses/bsd-license.php.
#
# 3) under the terms of the "GNU General Public License (GPL) Version 2" a
#    copy of which is available from the Open Source Initiative, see
#    http://www.opensource.org/licenses/gpl-license.php.
#
# Licensee has the right to choose one of the above licenses.
#
# Redistributions of source code must retain the above copyright
# notice and one of the license notices.
#
# Redistributions in binary form must reproduce both the above copyright
# notice, one of the license notices in the documentation
# and/or other materials provided with the distribution.


use strict;
use File::Basename;
use File::Path;
use File::Find;
use File::Copy;
use File::Glob qw/:bsd_glob/;
use File::Temp;
use Cwd;
use Term::ANSIColor qw(:constants);

my $WDIR = dirname(Cwd::abs_path $0);
require("$WDIR/common.pl");

use sigtrap 'handler', \&sig_handler, 'normal-signals';
my $PREREQUISIT = "172";
my $MST_START_FAIL = "173";
my $NO_HARDWARE = "171";
my $SUCCESS = "0";
my $DEVICE_INI_MISSING = "2";
my $ERROR = "1";
my $EINVAL = "22";
my $ENOSPC = "28";
my $NONOFEDRPMS = "174";
my $enable_mlnx_tune = 0;

$ENV{"LANG"} = "C";

$| = 1;
my $LOCK_EXCLUSIVE = 2;
my $UNLOCK         = 8;
#Setup some defaults
my $KEY_ESC=27;
my $KEY_CNTL_C=3;
my $KEY_ENTER=13;

my $BASIC = 1;
my $HPC = 2;
my $ALL = 3;
my $CUSTOM = 4;
my $VMA = 5;
my $VMAVPI = 6;
my $VMAETH = 7;
my $GUESTOS = 8;
my $HYPERVISOROS = 9;
my $BLUEFIELD = 10;
my $XLIO = 11;

my $interactive = 1;
my $kernel_only = 0;
my $quiet = 0;
my $verbose = 1;
my $verbose2 = 0;
my $verbose3 = 0;

my $print_available = 0;

my $clear_string = `clear`;
my $bonding_force_all_os = 0;

my $vendor_pre_install = "";
my $vendor_post_install = "";
my $vendor_pre_uninstall = "";
my $vendor_post_uninstall = "";

my $do_copy_udev = 0;
my $build_dkms_packages = 1;

# list of scripts to run for each package
my %package_pre_build_script = ();
my %package_post_build_script = ();
my %package_pre_install_script = ();
my %package_post_install_script = ();

my $DISTRO = "";
my $rpmbuild_flags = "";
my $rpminstall_flags = "";
my $rpminstall_parameter = "-i";
my $speed_up_kmp = 0;

chdir $WDIR;
my $CWD     = getcwd;
my $TMPDIR  = '/tmp';
my $netdir;

my $conf_dir = $CWD;
my $config = $conf_dir . '/ofed.conf';
chomp $config;
my $config_net;

my $builddir = "/var/tmp";
chomp $builddir;

my $PACKAGE     = 'OFED';

my $default_prefix = '/usr';
chomp $default_prefix;
my $prefix = $default_prefix;

my $arch = `uname -m`;
chomp $arch;
my $kernel = `uname -r`;
chomp $kernel;
my $kernel_sources = "/lib/modules/$kernel/build";
chomp $kernel_sources;

# Define RPMs environment
my $dist_rpm;
my $dist_rpm_ver = 0;
my $dist_rpm_rel = 0;

my $umad_dev_rw = 0;
my $umad_dev_na = 0;
my $config_given = 0;
my $config_net_given = 0;
my $kernel_given = 0;
my $kernel_source_given = 0;
my $install_option;
my $force_all = 0;
if (-e ".def_option" ) {
    $install_option = `cat .def_option 2>/dev/null`;
    chomp $install_option;
}
my $user_space_only = 0;
my $check_linux_deps = 1;
my $force = 0;
my $update = 0;
my $build_only = 0;
my $check_deps_only = 0;
my $print_distro = 0;
my $uninstall = 1;
my $kmp = 1;
my $kmp_build_num = "";
my %disabled_packages;
my %force_enable_packages;
my %packages_deps = ();
my %modules_deps = ();
my $knem_prefix = '';
my $mlnx_ethtool_prefix = "/opt/mellanox/ethtool";
my $mlnx_ethtool_mandir = "$mlnx_ethtool_prefix/share/man";
my $mlnx_ethtool_defaultdocdir = "$mlnx_ethtool_prefix/share/doc";
my $with_memtrack = 0;
my $with_gds = 1;
my $with_sf_cfg_drv = 0;
my $with_vma = 0;
my $with_xlio = 0;
my $with_fabric_collector = 0;
my $package_manager = "";
my $with_valgrind = 0;
my $disable_valgrind = 0;
my $with_mlx5_ipsec = 1;
my $have_numa_libs = 0;
my $with_ovs = 0;
my $with_openvswitch = 0;
my $with_pyverbs = 0;
my $enable_opensm = 0;
my @components = qw/kernel user/;
my $add_debuginfo_macros = 0;
my $rpm_macros_path = "";
my $rpm_macros_file; # Global for File::Temp scoping
my $kernel_extra_args = "";
my $with_fwctl = 0;
my $with_virtiofs = 0;
my $with_dkms_install = 1;

my $is_bf = `lspci -s 00:00.0 2> /dev/null | grep -wq "PCI bridge: Mellanox Technologies" && echo 1 || echo 0`;
chomp $is_bf;

my $with_bluefield = 0;
if  ($is_bf) {
    $with_bluefield = 1;
}

my $CMD = "$0 " . join(' ', @ARGV);
while ( $#ARGV >= 0 ) {

   my $cmd_flag = shift(@ARGV);

    if ( $cmd_flag eq "-c" or $cmd_flag eq "--config" ) {
        $config = shift(@ARGV);
        $interactive = 0;
        $config_given = 1;
    } elsif ( $cmd_flag eq "-n" or $cmd_flag eq "--net" ) {
        $config_net = shift(@ARGV);
        $config_net_given = 1;
    } elsif ( $cmd_flag eq "-l" or $cmd_flag eq "--prefix" ) {
        $prefix = shift(@ARGV);
        $prefix =~ s/\/$//;
    } elsif ( $cmd_flag eq "-k" or $cmd_flag eq "--kernel" ) {
        $kernel = shift(@ARGV);
        $kernel_given = 1;
    } elsif ( $cmd_flag eq "-s" or $cmd_flag eq "--kernel-sources" ) {
        $kernel_sources = shift(@ARGV);
        $kernel_source_given = 1;
    } elsif ( $cmd_flag eq "-U" or $cmd_flag eq "--update" ) {
        $update = 1;
    } elsif ( $cmd_flag eq "-b" or $cmd_flag eq "--build-only" ) {
        $build_only = 1;
    } elsif ( $cmd_flag eq "--check-deps-only" ) {
        $check_deps_only = 1;
    } elsif ( $cmd_flag eq "--print-distro" ) {
        $print_distro = 1;
    } elsif ( $cmd_flag eq "-p" or $cmd_flag eq "--print-available" ) {
        $print_available = 1;
    } elsif ( $cmd_flag eq "--force" ) {
        $force = 1;
    } elsif ( $cmd_flag eq "--disable-kmp" ) {
        $kmp = 0;
    } elsif ( $cmd_flag eq "--bump-kmp-version" ) {
        $kmp_build_num = shift(@ARGV);
    } elsif ( $cmd_flag eq "--speed-up-kmp" ) {
        $speed_up_kmp = 1;
    } elsif ( $cmd_flag eq "--all" ) {
        $interactive = 0;
        $install_option = 'all';
        $force_all = 1;
    } elsif ( $cmd_flag eq "--bluefield" ) {
        $interactive = 0;
        # Do not override other install options to enable bluefield packages as an extension
        $install_option = 'bluefield' if (not $install_option or ($install_option eq 'all' and not $force_all));
        $with_bluefield = 1;
    } elsif ( $cmd_flag eq "--hpc" ) {
        $interactive = 0;
        $install_option = 'hpc';
    } elsif ( $cmd_flag eq "--with-vma" and not ($install_option eq 'eth-only')) {
        $with_vma = 1;
    } elsif ( $cmd_flag eq "--vma" ) {
        $interactive = 0;
        $install_option = 'vma';
        $with_vma = 1;
    } elsif ( $cmd_flag eq "--vma-eth" ) {
        $interactive = 0;
        $install_option = 'vmaeth';
        $with_vma = 1;
    } elsif ( $cmd_flag eq "--vma-vpi" ) {
        $interactive = 0;
        $install_option = 'vmavpi';
        $with_vma = 1;
    } elsif ( $cmd_flag eq "--xlio" ) {
        $interactive = 0;
        $install_option = 'xlio';
        $with_xlio = 1;
    } elsif ($cmd_flag eq "--with-xlio") {
        $with_xlio = 1;
    } elsif ( $cmd_flag eq "--basic" ) {
        $interactive = 0;
        $install_option = 'basic';
    } elsif ( $cmd_flag eq "--guest" ) {
        $interactive = 0;
        $install_option = 'guest-os';
    } elsif ( $cmd_flag eq "--hypervisor" ) {
        $interactive = 0;
        $install_option = 'hypervisor-os';
    } elsif ( $cmd_flag eq "--kernel-only" ) {
        $interactive = 0;
        $kernel_only = 1;
        $install_option = 'kernel-only';
        @components = qw/kernel/;
    } elsif ( $cmd_flag eq "--user-space-only" ) {
        $user_space_only = 1;
        @components = qw/user/;
    } elsif ( $cmd_flag eq "--dpdk" ) {
        $interactive = 0;
        $install_option = 'dpdk';
    } elsif ( $cmd_flag eq "--ovs" ) {
        $with_ovs = 1;
    } elsif ( $cmd_flag eq "--with-pyverbs" ) {
        $with_pyverbs = 1;
    } elsif ( $cmd_flag eq "--umad-dev-rw" ) {
        $umad_dev_rw = 1;
    } elsif ( $cmd_flag eq "--umad-dev-na" ) {
        $umad_dev_na = 1;
    } elsif ( $cmd_flag eq "--eth-only" ) {
        $interactive = 0;
        $install_option = 'eth-only';
    } elsif ( $cmd_flag eq "--build32" ) {
        # Disabled
    } elsif ( $cmd_flag eq "--without-depcheck" ) {
        $check_linux_deps = 0;
    } elsif ( $cmd_flag eq "--with-memtrack" ) {
        $with_memtrack = 1;
    } elsif ( $cmd_flag eq "--enable-gds" ) {
        # Left for backward compatibility
    } elsif ( $cmd_flag eq "--builddir" ) {
        $builddir = shift(@ARGV);
        $builddir = clean_path($builddir);
    } elsif ( $cmd_flag eq "--copy-ifnames-udev" ) {
        $do_copy_udev = 1;
        $build_dkms_packages = 0; # abuse --copy-ifnames-udev option
    } elsif ( $cmd_flag eq "--without-dkms" ) {
        $build_dkms_packages = 0;
    } elsif ( $cmd_flag eq "--without-dkms-install" ) {
        $with_dkms_install = 0;
    } elsif ( $cmd_flag eq "--tmpdir" ) {
        $TMPDIR = shift(@ARGV);
        $TMPDIR = clean_path($TMPDIR);
    } elsif ( $cmd_flag eq "-q" ) {
        $quiet = 1;
    } elsif ( $cmd_flag eq "-v" ) {
        $verbose = 1;
    } elsif ( $cmd_flag eq "-vv" ) {
        $verbose = 1;
        $verbose2 = 1;
    } elsif ( $cmd_flag eq "-vvv" ) {
        $verbose = 1;
        $verbose2 = 1;
        $verbose3 = 1;
    } elsif ($cmd_flag eq "--conf-dir") {
	$conf_dir = shift(@ARGV);
	mkpath([$conf_dir]) unless -d "$conf_dir";
	if (not $config_given) {
		$config = $conf_dir . '/ofed.conf';
	}
    } elsif ($cmd_flag eq "--with-fabric-collector") {
        $with_fabric_collector = 1;
    } elsif ( $cmd_flag eq "--with-valgrind" ) {
        $with_valgrind = 1;
        $disable_valgrind = 0;
    } elsif ( $cmd_flag eq "--without-valgrind" ) {
        $with_valgrind = 0;
        $disable_valgrind = 1;
    } elsif ( $cmd_flag eq "--without-mlx5-ipsec") {
        $with_mlx5_ipsec = 0;
    } elsif ( $cmd_flag eq "--enable-mlnx_tune" ) {
        $enable_mlnx_tune = 1;
    } elsif ( $cmd_flag eq "--enable-opensm" ) {
        $enable_opensm = 1;
    } elsif ( $cmd_flag =~ /--without|--disable/ ) {
        my $pckg = $cmd_flag;
        $pckg =~ s/--without-|--disable-//;
        $disabled_packages{$pckg} = 1;
    } elsif ( $cmd_flag =~ /--with-|--enable-/ ) {
        my $pckg = $cmd_flag;
        $pckg =~ s/--with-|--enable-//;
        $force_enable_packages{$pckg} = 1;
        if ($pckg =~ /openvswitch/) {
            $with_openvswitch = 1;
        }
    } elsif ( $cmd_flag eq "--distro" ) {
        $DISTRO = shift(@ARGV);
    } elsif ( $cmd_flag =~ /--pre-build-/) {
        my $pckg = $cmd_flag;
        $pckg =~ s/--pre-build-//;
        my $script = shift(@ARGV);
        $package_pre_build_script{$pckg} = $script;
    } elsif ( $cmd_flag =~ /--post-build-/) {
        my $pckg = $cmd_flag;
        $pckg =~ s/--post-build-//;
        my $script = shift(@ARGV);
        $package_post_build_script{$pckg} = $script;
    } elsif ( $cmd_flag =~ /--pre-install-/) {
        my $pckg = $cmd_flag;
        $pckg =~ s/--pre-install-//;
        my $script = shift(@ARGV);
        $package_pre_install_script{$pckg} = $script;
    } elsif ( $cmd_flag =~ /--post-install-/) {
        my $pckg = $cmd_flag;
        $pckg =~ s/--post-install-//;
        my $script = shift(@ARGV);
        $package_post_install_script{$pckg} = $script;
    } elsif ( $cmd_flag eq "--package-install-options" ) {
        my $install_opt = shift(@ARGV);
        $install_opt =~ s/,/ /g;
        $rpminstall_flags .= " $install_opt";
    } elsif ( $cmd_flag eq "--kernel-extra-args" ) {
        $kernel_extra_args = shift(@ARGV);
    } elsif ( $cmd_flag eq "--fwctl" ) {
        $with_fwctl = 1;
    } elsif ( $cmd_flag eq "--virtiofs" ) {
        $with_virtiofs = 1;
    } elsif ( $cmd_flag eq "--help" or $cmd_flag eq "-h" ) {
        usage();
        exit 0;
    } elsif ( $cmd_flag eq "--script-version") {
        print_script_version();
        exit 0;
    } else {
        print RED "\nUnsupported installation option: '$cmd_flag'", RESET "\n";
        print "To see list of supported options, run: $0 --help\n";
        exit 1;
    }
}

my $modules_list = "$TMPDIR/mlnx_kos.$$";
unlink $modules_list;

if ($build_only and not (($install_option eq 'kernel-only') or ($install_option eq 'eth-only'))) {
    print RED "\nError: The '--build-only' option is supported only when '--kernel-only' option is given!", RESET "\n";
    print "To see list of supported options, run: $0 --help\n";
    exit 1;
}

if ($user_space_only and $kernel_only) {
    print RED "\nError: The options '--kernel-only' and '--user-space-only' are incompatible. Aborting.", RESET "\n";
    exit 1;
}

if ($with_ovs and $arch !~ /x86_64|aarch64/) {
    print YELLOW "\nWARNING: The '--ovs' option is supported only on x86_64 and aarch64. Disabling...", RESET "\n";
    $with_ovs = 0;
}

if ((not $build_only) and (not $print_available) and (not $print_distro)) {
    check_root_user();
}

for my $ver_file (qw{/etc/os-release /etc/issue}){
    next unless (-e $ver_file);

    my $format = '[%{NAME}]-[%{VERSION}]-[%{RELEASE}]\n';
    my $dist_rpm_str = `rpm -qf --queryformat '$format' $ver_file 2> /dev/null | grep -v "is not owned by any package" | head -1`;
    chomp $dist_rpm_str;
    next unless ($dist_rpm_str);

    $dist_rpm = $dist_rpm_str;
    last;
}
if (not defined $dist_rpm) {
    $dist_rpm = "unsupported";
}

#
# logging
#
my $ofedlogs = "$TMPDIR/$PACKAGE.$$.logs";
mkpath([$ofedlogs]);
my $glog = "$ofedlogs/general.log";
rmtree $glog;
open(GLOG, ">$glog") or die "Can't open $glog: $!\n";
close(GLOG);

sub print_and_log
{
    my $msg = shift @_;
    my $verb = shift @_;

    open(GLOG, ">>$glog") or die "Can't open $glog: $!\n";
    print GLOG "$msg";
    close(GLOG);

    if ($verb) {
        print "$msg";
    }
}

sub print_and_log_colored
{
    my $msg = shift @_;
    my $verb = shift @_;
    my $color = shift @_;

    open(GLOG, ">>$glog") or die "Can't open $glog: $!\n";
    print GLOG "$msg\n";
    close(GLOG);

    if ($verb) {
        if ($color eq "RED") {
            print RED "$msg", RESET "\n";
        } elsif ($color eq "YELLOW") {
            print YELLOW "$msg", RESET "\n";
        } elsif ($color eq "GREEN") {
            print GREEN "$msg", RESET "\n";
        } else {
            print "$msg\n";
        }
    }
}

# Sets the global $rpm_macros_path
sub set_rpm_macros_path() {
    return unless ($add_debuginfo_macros);

    my $macros_path = `rpmbuild --showrc | awk '/^Macro path:/ {print \$3}'`;
    chomp $macros_path;
    my $macros_text = <<'EOF';
# rpmmacros snippet for enabling debug info in SLES
# Generated by install.pl .

%_enable_debug_packages 1
# Some extra features that could be handy:
%_include_minidebuginfo 1
# Seems to have extra dependencies:
#_include_gdb_index 1
# Terminate build on missing build ids:
%_missing_build_ids_terminate_build 1

# Add the macro debug_package right before the install section.
# But only if:
# * It was not added before explicitly (__debug_package not defined)
# * It has not been disabled (_enable_debug_packages set)
%install %{!?__debug_package:%{?_enable_debug_packages:%{debug_package}}}\
%%install\
%{nil}
EOF
    print_and_log($macros_text, $verbose3);
    $rpm_macros_file = File::Temp->new(DIR=>$TMPDIR, CLEANUP=>1);
    print $rpm_macros_file $macros_text;
    $rpm_macros_path = $rpm_macros_file->filename . ":$macros_path";
}

print_and_log("Install command: $CMD\n", 0);

my $rpm_distro = '';

# don't auto-detect distro if it's provided by the user.
if ($DISTRO eq "") {
    print_and_log("Distro was not provided, trying to auto-detect the current distro...\n", $verbose2);
    print_and_log("dist_rpm: $dist_rpm\n", $verbose2);
    ($rpm_distro, $DISTRO) = parse_rpm_dist($dist_rpm, $kernel);

    if ($DISTRO eq "") {
        print_and_log_colored("Current operation system is not supported!", 1, "RED");
        exit 1;
    }

    print_and_log("Auto-detected $DISTRO distro.\n", $verbose2);
} else {
    print_and_log("Using provided distro: $DISTRO\n", $verbose2);

    $rpm_distro = $DISTRO;
    $rpm_distro = lc($rpm_distro);
    $rpm_distro =~ s/\./u/g;

    $DISTRO = uc ($DISTRO);
    if ($DISTRO =~ /SLES/) {
        $DISTRO =~ s/SP.*//g;
    } elsif ($DISTRO =~ /EULER/) {
        # Do nothing. Don't mangle OPENEULER.
        # FIXME: the one below is probably intended for openSUSE. Remove.
    } elsif ($DISTRO =~ /OPEN/) {
        $DISTRO =~ s/OPEN/open/g;
    }
}
if ($print_distro) {
    print_and_log("Printing distribution name and exiting:\n", $verbose2);
    print "$rpm_distro\n";
    exit 0;
}

sub warning($) {
    my $msg = shift @_;
    print_and_log_colored($msg, 1, "YELLOW");
}

if ($with_gds and $DISTRO !~ /RHEL[89] | KYLIN/x) {
    $with_gds = 0;
}

if ($DISTRO =~ /EULEROS2.0SP(8|10|11)/) { # Note: not SP9
    $with_sf_cfg_drv = 1;
}

my $SRPMS = $CWD . '/' . 'SRPMS/';
chomp $SRPMS;
my $RPMS  = $CWD . '/' . 'RPMS' . '/' . $dist_rpm . '/' . $arch;
chomp $RPMS;
if (not -d $RPMS) {
    mkpath([$RPMS]);
}

my $target_cpu  = `rpm --eval '%{_target_cpu}'`;
chomp $target_cpu;

$uninstall = 0 if ($update or $build_only);
$rpminstall_parameter = "-UF" if ($update);

if ($kernel_given and not $kernel_source_given) {
    if (-d "/lib/modules/$kernel/build") {
        $kernel_sources = "/lib/modules/$kernel/build";
    }
    else {
        print_and_log_colored("Provide path to the kernel sources for $kernel kernel.", 1, "RED");
        exit 1;
    }
}

my $kernel_rel = $kernel;
$kernel_rel =~ s/-/_/g;

my $cuda_version = "13-2";
if ($DISTRO =~ /RHEL7/) {
	$cuda_version = "12-2";
}
if ($DISTRO =~ /TENCENTOS4\.4/) {
    $cuda_version = "12-8";
}
my $cuda_dir_name = $cuda_version;
$cuda_dir_name =~ y/-/./;
my $cuda_top_dir = "";

if ($DISTRO =~ m/WINDRIVER6|BLUENIX1/) {
    $rpmbuild_flags .= ' --nodeps';
    $rpminstall_flags .= ' --nodeps';
}

if (not $check_linux_deps) {
    $rpmbuild_flags .= ' --nodeps';
    $rpminstall_flags .= ' --nodeps';
}
my $optflags  = `rpm --eval '%{optflags}'`;
chomp $optflags;

my $mandir      = `rpm --eval '%{_mandir}'`;
chomp $mandir;
my $sysconfdir  = "/etc";
chomp $sysconfdir;
my %main_packages = ();
my @selected_packages = ();
my @selected_by_user = ();
my @selected_modules_by_user = ();
my @packages_to_uninstall = ();
my @dependant_packages_to_uninstall = ();
my %selected_for_uninstall = ();
my %non_ofed_for_uninstall = ();
my @selected_kernel_modules = ();

my $ibutils2_prefix = '/usr';
my $compiler = "gcc";

if ($install_option eq 'guest-os') {
    $rpminstall_flags .= ' --nodeps ';
}

if ($DISTRO =~ /SLES|SUSE/) {
    $package_manager = "zypper";
} elsif ($DISTRO =~ /BLUENIX/) {
    $package_manager = "smart";
} elsif ($DISTRO =~ /AZURE/) {
    $package_manager = "dnf";
} else {
    $package_manager = "yum";
}

my $pmi_opt = '';
#if (is_installed("slurm")) {
#    $pmi_opt = '--with-pmi';
#}

my $gcc = "gcc";
my $gcc_cpp = "gcc-c++";
my $libstdc = '';
my $libgcc = 'libgcc';
my $libgfortran = '';
my $fortran = 'gcc-gfortran';
my $curl_devel = 'curl-devel';
my $libnl_devel = 'libnl-devel';
my $libnl3_devel = 'libnl3-devel';
my $libdb_devel = 'libdb-devel';
my $elfutils_devel = 'elfutils-devel';
my $kernel_elfutils_devel = 'elfutils-libelf-devel';
my $iptables_devel = 'iptables-devel';
my $libmnl = 'libmnl';
my $libnl = 'libnl';
my $libnl3 = 'libnl3';
my $glib2 = 'glib2';
my $glib2_devel = 'glib2-devel';
my $glibc_devel = 'glibc-devel';
my $openssl_devel = 'openssl-devel';
my $openssl = 'openssl';
my $libcurl = 'libcurl';
my $python = "python";
my $python_unrevisioned = "";
my $python_devel = "python-devel";
my $python3_devel = "python3-devel";
my $python_cython = "";
my $python_docutils = "";
my $pkgconfig = "pkgconfig";
my $tcl_devel = 'tcl-devel';
my $zlib_devel = 'zlib-devel';
my $srvtool = "";
my $cmake = "cmake__2.8.11";
my $pciutils_libs = "pciutils-libs";
my $libusb = "libusbx";
my $libusb_devel = "libusbx-devel";
my $fuse_libs = "fuse-libs";
my $python_rpm_macros = "";
my $gcc_toolset_11 = "";
my $gcc_toolset_11_annobin = "";
my $annobin_gcc = "";

if ($DISTRO =~ /OL8.[7-9]|OL8.10/) {
    $gcc_toolset_11 = "gcc-toolset-11";
    $gcc_toolset_11_annobin = "gcc-toolset-11-annobin-plugin-gcc";
} elsif ($DISTRO =~ /OL9.6/) {
    $gcc_toolset_11 = "gcc-toolset-14";
    $gcc_toolset_11_annobin = "gcc-toolset-14-annobin-plugin-gcc";
}

if ($DISTRO =~ /ALINUX3.12/) {
    $annobin_gcc = "gcc-toolset-12-annobin-plugin-gcc";
}

if ($DISTRO =~ /OPENEULER|EULEROS2.0SP(9|1[0-9])|RHEL[89]|FC3|BCLINUX8|BCLINUX2[12]|KYLIN11$|OL[89]|ANOLIS|CTYUNOS/) {
    $python_docutils = "python3-docutils";
} elsif ($DISTRO =~ /RHEL7|BCLINUX7|OL7/) {
    $python_docutils = "python-docutils";
}

if ($DISTRO =~ /OPENEULER22|CTYUNOS2[35]/) {
    $python_unrevisioned = "python3-unversioned-command";
}

if ($DISTRO =~ /UOS/) {
    $python = "python2";
    $python_devel = "python2-devel";
    $pkgconfig = "pkgconf";
    $fuse_libs = "fuse";
    $kernel_elfutils_devel = "elfutils-devel";
    $pciutils_libs = "pciutils";
    $libnl = "libnl3";
    $libnl_devel = "libnl3-devel";
}

if ($DISTRO eq "openSUSE_TUMBLEWEED") {
    $libstdc = 'libstdc++6';
    $pkgconfig = "python-pkgconfig";
    $libgcc = 'libgcc_s1';
    $glib2 = "glib2-tools";
} elsif ($DISTRO eq "openSUSE12.1") {
    $libstdc = 'libstdc++46';
    $libgcc = 'libgcc46';
    $libgfortran = 'libgfortran46';
    $fortran = 'libgfortran46';
    $curl_devel = 'libcurl-devel';
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3-200';
    $glib2 = "glib2-tools";
    $libcurl = "libcurl4";
    $pkgconfig = "pkg-config";
} elsif ($DISTRO =~ m/SLES12/) {
    $libstdc = 'libstdc++6';
    $libgcc = 'libgcc_s1';
    $libgfortran = 'libgfortran3';
    $curl_devel = 'libcurl-devel';
    $fortran = "gcc-fortran";
    $openssl_devel = 'libopenssl-devel';
    $openssl = 'libopenssl1_0_0';
    $libcurl = "libcurl4";
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3-200';
    $glib2 = 'libglib-2_0-0';
    $pkgconfig = "pkg-config";
    $libdb_devel = 'libdb-4_8-devel';
    $elfutils_devel = 'libelf-devel';
    $kernel_elfutils_devel = 'libelf-devel';
    $iptables_devel = 'libxtables-devel';
    $libmnl = 'libmnl0';
} elsif ($DISTRO =~ m/SLES15|openSUSE15/) {
    $srvtool = "insserv-compat";
    $libstdc = 'libstdc++6-devel-gcc7';
    $libgcc = 'libgcc_s1';
    $curl_devel = 'libcurl-devel';
    $fortran = "gcc-fortran";
    $openssl_devel = 'libopenssl-devel';
    $openssl = 'libopenssl1_1';
    $libcurl = "libcurl4";
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3-200';
    $glib2 = 'libglib-2_0-0';
    $pkgconfig = "pkg-config";
    $libdb_devel = 'libdb-4_8-devel';
    $elfutils_devel = 'libelf-devel';
    $kernel_elfutils_devel = 'libelf-devel';
    $libmnl = 'libmnl0';
    $iptables_devel = 'libxtables-devel';
    $libgfortran = "libgfortran4";
    $python = "python3";
    $python_devel = "python3-devel";
    $python_cython = "python3-Cython";
} elsif ($DISTRO =~ m/SLES16/) {
    $libstdc = 'libstdc++6-devel-gcc15';
    $libgcc = 'libgcc_s1';
    $curl_devel = 'libcurl-devel';
    $fortran = "gcc-fortran";
    $openssl_devel = 'libopenssl-devel';
    $openssl = 'libopenssl3';
    $libcurl = "libcurl4";
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3-200';
    $glib2 = 'libglib-2_0-0';
    $pkgconfig = "pkgconf-pkg-config";
    $libdb_devel = 'libdb-4_8-devel';
    $elfutils_devel = 'libelf-devel';
    $kernel_elfutils_devel = 'libelf-devel';
    $libmnl = 'libmnl0';
    $iptables_devel = 'libxtables-devel';
    $libgfortran = "libgfortran5";
    $python = "python313";
    $python_devel = "python313-devel";
    $python_cython = "python313-Cython";
} elsif ($DISTRO =~ m/RHEL|OL|FC|POWERKVM|EULER|BCLINUX|ALIOS|ANOLIS/) {
    $libstdc = 'libstdc++';
    $libgcc = 'libgcc';
    $libgfortran = 'gcc-gfortran';
    $curl_devel = 'libcurl-devel';
    if ($DISTRO =~ /FC19|FC[2-9]|OL[7-9]|RHEL[7-9]|POWERKVM|EULER|BCLINUX|ALIOS|ANOLIS/) {
        $openssl = 'openssl-libs';
    }
    if ($DISTRO =~ /RHEL6\.|OL6\./) {
        $libdb_devel = 'db4-devel';
        $libnl3 = '';
        $libnl3_devel = '';
    }
    if ($DISTRO =~ m/EULER/) {
        $libnl_devel = 'libnl3-devel';
    }
}else {
    $libstdc = 'libstdc++';
}

if ($DISTRO =~ /(SLES|openSUSE)1[256] | openSUSE_TUMBLEWEED/x) {
    $libnl3 = 'libnl3-200';
    $pciutils_libs = "libpci3";
    $libusb = "libusb-1_0-0";
    $libusb_devel = "libusb-1_0-devel";
    $fuse_libs = "libfuse2";
}

my $libcrypt = "";
if ($DISTRO =~ /RHEL9|OL9|RHEL10|AL23/) {
    $libcrypt = "libxcrypt-compat";
}

my $libstdc_devel = "$libstdc-devel";
my $libexpat = "expat";
my $libexpat_devel = "";
my $tk = "tk";
my $insserv_compat = "";

if ($DISTRO =~ m/SLES12/) {
    $libstdc_devel = 'libstdc++-devel';
} elsif ($DISTRO =~ m/SLES15|openSUSE15/){
    $libstdc_devel = "libstdc++6-devel-gcc7";
    $insserv_compat = "insserv-compat";
} elsif ($DISTRO =~ m/SLES16/){
    $libstdc_devel = "libstdc++6-devel-gcc15";
} elsif ($DISTRO eq "openSUSE_TUMBLEWEED") {
    $libstdc_devel = 'libstdc++6-devel-gcc5';
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3-200';
} elsif ($DISTRO =~ m/RHEL6|RHEL7|OL6|OL7|EULER|BCLINUX7|ALIOS/) {
    $libexpat_devel = "expat-devel";
} elsif ($DISTRO =~ m/WINDRIVER6/) {
    $gcc = "toolchain-wrappers";
    $gcc_cpp = "toolchain-wrappers";
    $python = "python-core";
    $python_devel = "python-dev";
    $libgcc = 'libgcc1';
    $libnl_devel = 'libnl-3-dev';
    $libnl = 'libnl-3-200';
    $libstdc = "libstdc++6";
    $libstdc_devel = "libstdc++-dev";
    $libexpat = "libexpat1";
    $libexpat_devel = "libexpat-dev";
    $tk = "";
    $curl_devel = 'curl-dev';
    $glib2 = 'libglib-2.0-0';
    $glib2_devel = 'libglib-2.0-dev';
    $glibc_devel = 'libc6-dev';
    $tcl_devel = 'tcl-dev';
    $zlib_devel = 'libz-dev';
} elsif ($DISTRO =~ m/POWERKVM | FC2[2-9] | FC3./x) {
    $libexpat_devel = "expat-devel";
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3';
} elsif ($DISTRO =~ m/BLUENIX1/) {
    $gcc_cpp = "g++";
    $python = "python-core";
    $python_devel = "python-dev";
    $libgcc = 'libgcc1';
    $libnl_devel = 'libnl-3-dev';
    $libnl = 'libnl-3-200';
    $libstdc = "libstdc++6";
    $libstdc_devel = "libstdc++-dev";
    $libexpat = "libexpat1";
    $libexpat_devel = "libexpat-dev";
    $tk = "";
    $curl_devel = 'curl-dev';
    $glib2 = 'libglib-2.0-0';
    $glib2_devel = 'libglib-2.0-dev';
    $glibc_devel = 'libc6-dev';
    $tcl_devel = 'tcl-dev';
    $zlib_devel = 'libz-dev';
} else {
    $libexpat_devel = "expat-devel";
}

if ($DISTRO =~ /SLES15/) {
    $add_debuginfo_macros = 1;
    set_rpm_macros_path();
    $rpmbuild_flags .= " --macros='$rpm_macros_path'";
}

my $perl_generators = "";
if ($DISTRO =~ /RHEL[89] | OL[89] | BCLINUX8 | ^FC | ANOLIS | CTYUNOS | KYLIN | TENCENT | ALINUX | AL23/x) {
    $perl_generators = "perl-generators";
}

my $libudev = "libudev";
my $libudev_devel = "libudev-devel";
if ($DISTRO =~ m/SLES12|SLES15|SLES16|openSUSE15/) {
    $libudev = "libudev1";
} elsif ($DISTRO =~ m/RHEL[7-9]|OL|FC|EULER|POWERKVM|BCLINUX|ALIOS|XenServer[78].[12]|UOS|ANOLIS|CTYUNOS|TENCENT|ALINUX/) {
    $libudev = "systemd-libs";
    $libudev_devel = "systemd-devel";
} elsif ($DISTRO =~ m/WINDRIVER6/) {
    $libudev = "libudev0";
    $libudev_devel = "udev-dev";
} elsif ($DISTRO =~ m/BLUENIX1|XenServer/) {
    $libudev = "";
    $libudev_devel = "";
}

if ($rpm_distro =~ m/sles15sp[4-7]|sles16sp0/) {
    $libudev_devel = "systemd-devel";
    $python_rpm_macros = "python-rpm-macros";
}

if ($DISTRO =~ /MARINER|AZURE|PHOTON/) {
    $libusb = "libusb";
    $libusb_devel = "libusb-devel";
    $python = "python3";
    $python_devel = "python3-devel";
    $python_cython = "python3-Cython";
    $glib2 = "glib";
    $glib2_devel = "glib-devel";
    $fuse_libs = "fuse-devel";
    $pciutils_libs = "pciutils-devel";
    $pkgconfig = "pkg-config";
    $fortran = "gfortran";
    $libnl = "libnl3";
    $libnl_devel = "libnl3-devel";
    $gcc_cpp = "";
    $libudev = "libgudev";
    $libudev_devel = "libgudev-devel";
}

if ($DISTRO =~ /MARINER2|AZURE/) {
    $pkgconfig = "pkgconf-pkg-config";
}

# PhotonOS have libnl-3.X which contains libnl-3.so
if ($DISTRO =~ /PHOTON/) {
    $libnl = "libnl";
    $libnl_devel = "libnl-devel";
    $libnl3 = "libnl";
    $libnl3_devel = "libnl-devel";
}

my $kernel_modules_extra = "";
if ($DISTRO =~ m/RHEL8/ and $DISTRO !~ /RHEL8.6/) {
    $kernel_modules_extra = "kernel-modules-extra";
}


my $systemd_devel = "systemd-devel";
if ($DISTRO =~ m/RHEL6 | OL6 | BLUENIX1 | WINDRIVER6/x) {
    $systemd_devel = "";
}

my $nvidia_driver_devel = "nvidia-driver-cuda-libs";
if ($DISTRO =~ /RHEL7/) {
    $nvidia_driver_devel = "nvidia-driver-latest-dkms-devel";
}

if ($DISTRO =~ m/FC2[6-9] | FC3./x) {
    $srvtool = "chkconfig";
    $python = "python3";
    $python_devel = "python3-devel";
    $python_cython = "python3-Cython";
    $pkgconfig = "pkgconf-pkg-config";
}

if ($DISTRO =~ /FC35/) {
    $libusb = "libusb1";
    $libusb_devel = "libusb1-devel";
}

if ($DISTRO =~ m/RHEL8 | OL8 | ANOLIS | TENCENT | ALINUX/x) {
    $python = "python36";
    $python_devel = "python36-devel";
    $python3_devel = "python36-devel";
    $python_cython = "python3-Cython";
    if ($DISTRO =~ m/ANOLIS8.4/x) {
        $python_devel = "python38-devel";
        $python3_devel = "python38-devel";
    }
    if ($DISTRO =~ m/ANOLIS8.4|ALINUX/x) {
        $python_cython = "python38-Cython";
    }
    if ($DISTRO =~ m/TENCENTOS4.4/x) {
        $python = "python3.12";
        $python_devel = "python3.12-devel";
        $python3_devel = "python3.12-devel";
    }
    $pkgconfig = "pkgconf-pkg-config";
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3';
}

if ($DISTRO =~ m/RHEL9|OL9/x) {
    $python = "python3";
    $python_devel = "python3-devel";
    $python3_devel = "python3-devel";
    $python_cython = "python3-Cython";
    $pkgconfig = "pkgconf-pkg-config";
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3';
}

if ($DISTRO =~ m/EULEROS2.0SP8/x) {
    $python = "python2";
    $python_devel = "python2-devel";
    $python_cython = "";
    $pkgconfig = "pkgconf-pkg-config";
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3';
}

if ($DISTRO =~ m/KYLIN/x) {
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3';
    $libudev = "systemd-libs";
    $libudev_devel = "systemd-devel";
    $python = "python3";
    $python_devel = "python3-devel";
    $fuse_libs = "fuse";
    $pkgconfig = "pkgconf";
    $pciutils_libs = "pciutils";
    $kernel_elfutils_devel = "elfutils-devel";
    $python_cython = "python3-Cython";

}

if ($DISTRO =~ m/OPENEULER | EULEROS2.0SP9 | EULEROS2.0SP1[0-9] | BCLINUX2[12] | CTYUNOS/x) {
    $python = "python3";
    $python_devel = "python3-devel";
    $pkgconfig = "pkgconf";
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3';
    $fuse_libs = "fuse-devel";
    $pciutils_libs = "pciutils-devel";
    $kernel_elfutils_devel = 'elfutils-devel';
    if ($DISTRO =~ /OPENEULER|CTYUNOS|BCLINUX2[12]/) {
        $python_cython = "python3-Cython";
    }
}

my $update_initrd_cmd = "dracut -f";

# build with libnl3 on RHEL7, the libnl3-devel here comes from Optional packages
if ($DISTRO =~ m/RHEL7|OL7|BCLINUX7|ALIOS/) {
    if ($with_pyverbs and ($DISTRO =~ /RHEL7/)) {
        $python = "python3";
        $python_devel = "python3-devel";
        $python_cython = "python36-Cython";
        $cmake = "cmake3";
    }
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3';
}

if ($DISTRO =~ m/BCLINUX8/) {
    $python = "python36";
    $python_devel = "python36-devel";
    $python_cython = "python3-Cython";
    if ($DISTRO =~ m/BCLINUX8.2/) {
        $python_devel = "python38-devel";
        $python_cython = "python38-Cython";
    }
    $libnl_devel = 'libnl3-devel';
    $libnl = 'libnl3';
    $pkgconfig = "pkgconf-pkg-config";
}

if ($DISTRO =~ m/FC3./x) {
    $insserv_compat = "chkconfig";
}

my $python_six = "python-six";
if ($DISTRO =~ m/FC27/) {
    $python_six = "python2-six";
} elsif ($DISTRO =~ /RHEL[89] | RHEL10 | OL[89] | FC3[0-9] | EULEROS2.0SP1[01] | ANOLIS | OPENEULER | ALINUX | MARINER | CTYUNOS/x) {
    $python_six = "python3-six";
}

my $python_sphinx = "python-sphinx";
if ($DISTRO =~ /RHEL[89] | RHEL10 | OL[89] | FC3[0-9] | EULEROS2.0SP1[01] | ANOLIS | OPENEULER | ALINUX | CTYUNOS/x) {
    $python_sphinx = "python3-sphinx";
}

my $python_twisted = "python-twisted-core";
if ($DISTRO =~ /MARINER | RHEL8 | OL[89] | FC3[0-9] | OPENEULER/x) {
    $python_twisted = "python3-twisted";
} elsif ($DISTRO =~ /RHEL9|RHEL10|ANOLIS|ALINUX|CTYUNOS/) {
    $python_twisted = "";
}

my $python_zope_interface = "python-zope-interface";
if ($DISTRO =~ /MARINER | RHEL8 | RHEL10 | OL[89] | FC3[0-9] | EULEROS2.0SP1[01] | OPENEULER | CTYUNOS/x) {
    $python_zope_interface = "python3-zope-interface";
} elsif ($DISTRO =~ /RHEL9|ANOLIS|ALINUX/x) {
    $python_zope_interface = "";
}

if ($DISTRO =~ /FC3[1-9]|RHEL9.1|OL9|ANOLIS|ALINUX/) {
    $python_rpm_macros = "python-srpm-macros";
}

if ($DISTRO =~ /RHEL10 | FC4/x) {
    $libusb = "libusb1";
    $libusb_devel = "libusb1-devel";
    $pkgconfig = "pkgconf";
    $python = "python3";
    $python_devel = "python3-devel";
    $libdb_devel = "";
    $elfutils_devel = "elfutils-devel";
    $libnl = "libnl3";
    $libnl_devel = "libnl3-devel";
    $libudev = "systemd-libs";
    $libudev_devel = "systemd-devel";
    $perl_generators = "perl-generators";
    $python_cython = "python3-cython";
    $zlib_devel = 'zlib-ng-compat-devel';
}

if ($DISTRO =~ /AL2/) {
    if ($DISTRO =~ /AL2$/) {
        $libstdc_devel = 'libstdc++10-devel';
    }

    $libudev = "systemd-libs";
    $libudev_devel = "systemd-devel";

    $python = "python3";

    if ($DISTRO =~ /AL23/) {
        $libnl = "libnl3";
        $libnl_devel = "libnl3-devel";
        $python_devel = "python3-devel";
        $pkgconfig = "pkgconf-pkg-config";
        $python_cython = "python3-Cython"
    }
}

my $perl_srpm_macros = "";
if ($DISTRO =~ /RHEL9|OL9/) {
    $perl_srpm_macros = "perl-srpm-macros";
}

my $valgrind_devel = (($DISTRO =~ m/RHEL/ and not $disable_valgrind) or $with_valgrind) ? "valgrind-devel" : "";
if ($DISTRO =~ /RHEL8\.10/) {
    $valgrind_devel = "";
}

# Suffix for 32 and 64 bit packages
my $is_suse_suff64 = $arch eq "ppc64" && $DISTRO !~ /SLES12/;
my $suffix_64bit = ($DISTRO =~ m/SLES|openSUSE/ &&  $is_suse_suff64) ? "-64bit" : "";

sub usage
{
   print GREEN;
   print "\n";
   print "Usage: $0 [-c <packages config_file>|--all|--hpc|--vma|--xlio|--basic|--bluefield] [OPTIONS]\n";

   print "\n";
   print "Installation control:\n";
   print "    --force              Force installation\n";
   print "    --tmpdir             Change tmp directory. Default: $TMPDIR\n";
   print "    -l|--prefix          Set installation prefix\n";
   print "    -k|--kernel <version>\n";
   print "                         Default on this system: $kernel\n";
   print "    -s|--kernel-sources <path>\n";
   print "                         Default on this system: $kernel_sources\n";
   print "    -U|--update          Update installed version\n";
   print "    -b|--build-only      Build binary RPMs without installing them\n";
   print "                         - This option is supported only when '--kernel-only' option is given.\n";
   print "    --distro             Set Distro name for the running OS (e.g: rhel6.5, sles12sp3)\n";
   print "                         Default: Use auto-detection\n";
   print "    --without-depcheck   Run the installation without verifying that all required Distro's packages are installed\n";
   print "    --check-deps-only    Check for missing required Distro's packages and exit\n";
   print "    --print-distro       Print distribution name and exit\n";
   print "    --disable-kmp        Build kernel RPMs without KMP support\n";
   print "    --speed-up-kmp       Speed up KMP RPMs installation by skipping %post section and running the required operations once for all KMPs\n";
   print "    --builddir           Change build directory. Default: $builddir\n";
   print "    --umad-dev-rw        Grant non root users read/write permission for umad devices instead of default\n";
   print "    --umad-dev-na        Prevent from non root users read/write access for umad devices. Overrides '--umad-dev-rw'\n";
   print "    --enable-mlnx_tune   Enable Running the mlnx_tune utility\n";
   print "    --enable-opensm      Run opensm upon boot\n";
   print "    --without-mlx5-ipsec Disable IPsec support on ConnectX adapters\n";
   print "\n";
   print "    --package-install-options\n";
   print "                         RPM install options to use when installing RPM packages (comma separated list)\n";
   print "    --pre-build-<package> <path to script>\n";
   print "                         Run given script before given package's build\n";
   print "    --post-build-<package> <path to script>\n";
   print "                         Run given script after given package's build\n";
   print "    --pre-install-<package> <path to script>\n";
   print "                         Run given script before given package's install\n";
   print "    --post-install-<package> <path to script>\n";
   print "                         Run given script after given package's install\n";
   print "\n";
   print "Package selection:\n";
   print "    -c|--config <packages config_file>\n";
   print "                         Example of the config file can be found under docs (ofed.conf-example)\n";
if (not $install_option eq 'eth-only') {
   print "    --all                Install all available packages\n";
   print "    --bluefield          Install BlueField packages\n";
   print "    --hpc                Install minimum packages required for HPC\n";
   print "    --basic              Install minimum packages for basic functionality\n";
} else {
   print "    --eth-only           Install Ethernet drivers only\n";
}
   print "    --dpdk               Install minimum packages required for DPDK\n";
   print "    --ovs   Install OpenVSwitch packages\n";
if (not $install_option eq 'eth-only') {
   print "    --with-vma           Enable installing and configuring VMA package (to be used with any of the above installation options)\n";
}
   print "    --vma|--vma-vpi      Install minimum packages required by VMA to support VPI\n";
   print "    --vma-eth            Install minimum packages required by VMA to work over Ethernet\n";
   print "    --xlio     Install minimum packages required by XLIO\n";
if (not $install_option eq 'eth-only') {
   print "    --guest              Install minimum packages required by guest OS\n";
   print "    --hypervisor         Install minimum packages required by hypervisor OS\n";
   print "    --with-fabric-collector\n";
   print "                         Enable installing fabric-collector package\n";
   print "User-Space and libraries selection:\n";
   print "    --with-pyverbs       Build rdma-core package with pyverbs).\n";
   print "    --without-dkms       Build binary kernel modules instead of DKMS.\n";
   print "    --without-dkms-install    Build DKMS packages, but don't install them.\n";
}
   print "Extra package filtering:\n";
if (not $install_option eq 'eth-only') {
   print "    --kernel-only        Install kernel space packages only\n";
   print "    --user-space-only    Filter selected packages and install only User Space packages\n";
   print "    --fwctl              Enable FW control support\n";
   print "    --virtiofs           Enable virtiofs support\n";
}
   print "    --without-<package>  Do not install package\n";
   print "    --with-<package>     Force installing package\n";
   print "    --with-memtrack      Build ofa_kernel RPM with memory tracking enabled for debugging\n";
   print "    --kernel-extra-args '<args>'\n";
   print "                         pass <args> to kernel configure script (single paraeter, space separated)\n";
   print "\n";
   print "Miscellaneous:\n";
   print "    -h|--help            Display this help message and exit\n";
   print "    -p|--print-available Print available packages for current platform\n";
   print "                         And create corresponding ofed.conf file\n";
   print "    --conf-dir           Destination directory to save the configuration file\n";
   print "                         Default: $CWD\n";
   print "    --copy-ifnames-udev  Copy compatibility udev rules for interface names\n";
   print "\n";
   print "Output control:\n";
   print "    -v|-vv|-vvv          Set verbosity level\n";
   print "    -q                   Set quiet - no messages will be printed\n";
   print RESET "\n\n";
}

my $sysfsutils;
my $sysfsutils_devel;

if ($DISTRO =~ m/SLES|openSUSE/) {
    $sysfsutils = "sysfsutils";
    $sysfsutils_devel = "sysfsutils";
} elsif ($DISTRO =~ m/RHEL6|RHEL7|OL6|OL7|POWERKVM|EULEROS2.0SP[235]|BCLINUX7|ALIOS/) {
    $sysfsutils = "libsysfs";
    $sysfsutils_devel = "libsysfs";
} elsif ($DISTRO =~ m/BLUENIX1/) {
    $sysfsutils = "sysfsutils";
    $sysfsutils_devel = "sysfsutils-dev"
}

my @kernel_req = ();
my $kernel_devel = "kernel-devel-$kernel";
if ($DISTRO =~ /RHEL8 | OL8 | EULEROS2.0SP8 | EULEROS2.0SP1[23] | BCLINUX8 | FC29 | FC3. | ANOLIS | ALINUX | TENCENT | AL23/x) {
    @kernel_req = qw/kernel-rpm-macros/;
} elsif ($DISTRO =~ /RHEL9|RHEL10|OL9/) {
    @kernel_req = qw/kernel-srpm-macros kernel-rpm-macros/;
} elsif ($DISTRO =~ /RHEL|OL|FC|XenServer|EULEROS2.0SP[235]|BCLINUX7.[3-4]|ALIOS/) {
    @kernel_req = qw/redhat-rpm-config/;
} elsif ($DISTRO =~ /BCLINUX7.[5-9]/) {
    @kernel_req = qw/bclinux-rpm-config/;
} elsif ($DISTRO =~ /EULEROS2.0SP9 | EULEROS2.0SP1[01]/x) {
    @kernel_req = ("openEuler-rpm-config", "kernel-rpm-macros.$arch");
} elsif ($DISTRO =~ /OPENEULER/x) {
    @kernel_req = qw/openEuler-rpm-config/;
} elsif ($DISTRO =~ /CTYUNOS/) {
    @kernel_req = qw/ctyunos-rpm-config/;
} elsif ($DISTRO =~ /POWERKVM/) {
    @kernel_req = qw/base-rpm-config/;
} elsif ($DISTRO =~ /SLES/) {
    my $flavor = "default";
    if ($kernel =~ /^\d+\.\d+\.\d+-\d+-(azure|preempt)/) {
        $flavor = $1;
    }
    my $kd = "kernel-$flavor-devel";
    @kernel_req = ($kd);
    $kernel_devel = $kd;
} elsif ($DISTRO =~ /WINDRIVER6|BLUENIX1/) {
    @kernel_req = qw/kernel-dev/;
    $kernel_devel = "kernel-dev";
}
if ($kernel =~ /uek/) {
    $kernel_devel = "kernel-uek-devel-$kernel";
}

my $network_dir;
if ($DISTRO =~ m/SLES/) {
    $network_dir = "/etc/sysconfig/network";
}
elsif ($DISTRO =~ m/BLUENIX1/) {
    $network_dir = "/etc/network";
} else {
    $network_dir = "/etc/sysconfig/network-scripts";
}

my $systemd_rpm_macros = "systemd";
if ($DISTRO =~ /SLES/) {
    $systemd_rpm_macros = "systemd-rpm-macros";
}

# List of packages that were included in the previous OFED releases
# for uninstall purpose
my $openvswitch = "";
my @openvswitch_packages = ();
if (($with_bluefield or $with_ovs or $with_openvswitch) and
        $DISTRO =~ /RHEL[7-9] | RHEL10 |FC3[0-9] | OPENEULER20.03SP1 | ANOLIS | ALINUX | CTYUNOS/x) {
    $openvswitch = "openvswitch";
    my $python_ovs_pkg = "python-openvswitch";
    if ($DISTRO =~ /RHEL[89] | RHEL10 | OL[89] | FC3[0-9] | OPENEULER20.03SP1 | ANOLIS | ALINUX | CTYUNOS/x) {
        $python_ovs_pkg = "python3-openvswitch";
    }
    @openvswitch_packages = (
        "openvswitch", "openvswitch-devel", "openvswitch-ipsec", "openvswitch-selinux-policy", $python_ovs_pkg, "openvswitch-test"
    );
}


my @distro_ofed_packages = (
                        "libamso", "libamso-devel", "dapl2", "dapl2-devel", "mvapich", "mvapich2", "mvapich2-devel",
                        "mvapich-devel", "libboost_mpi1_36_0", "libmthca-rdmav2", "libcxgb3-rdmav2", "libcxgb4-rdmav2",
                        "libmlx4-rdmav2", "libibmad1", "libibumad1", "libibcommon1", "libibdm1", "ofed", "ofa",
                        "libibumad3", "libibmad5",
                        "rdma-ofa-agent", "rdma", "rdma-core", "rdma-core-devel", "rsocket", "libosmcomp3",
                        );

my @mlnx_en_packages = (
                       "mlnx_en", "mlnx-en-devel", "mlnx_en-devel", "mlnx_en-doc", "mlnx-ofc", "mlnx-ofc-debuginfo"
                        );

# List of all available packages sorted following dependencies
if ($kmp) {
    system("rpm -qf $kernel_sources/scripts > /dev/null 2>&1");
    my $res = $? >> 8;
    my $sig = $? & 127;
    if ($sig or $res) {
        print_and_log("KMP is not supported for kernels which were not installed as RPM.\n", $verbose2);
        $kmp = 0;
    }
}

if ($kmp and ($DISTRO =~ m/XenServer|FC|WINDRIVER6|POWERKVM|BLUENIX1|KYLIN|BCLINUX2[12]|UOS|OPENEULER20.03SP[13]|OPENEULER2[24].03|PHOTON|MARINER|AZURE|CTYUNOS|TENCENT|OL9.4/ or $kernel =~ /xs|fbk|fc|debug/)) {
    print_and_log_colored("KMP is not supported on $DISTRO. Switching to non-KMP mode", $verbose2, "RED");
    $kmp = 0;
}

# Disable KMP for kernels incompatible with the original Distro's kernel
if ($kmp) {
    # RHEL
    if (($DISTRO eq "EULEROS2.0SP2" and $kernel !~ /3.10.0-327.*$arch/) or
        ($DISTRO eq "EULEROS2.0SP3" and $kernel !~ /3.10.0-514.*$arch/) or
        ($DISTRO eq "EULEROS2.0SP5" and $kernel !~ /3.10.0-862\..*eulerosv2r7\.$arch/) or
        ($DISTRO eq "EULEROS2.0SP8" and $kernel !~ /4.19.36-vhulk1907\..*\.$arch/) or
        ($DISTRO eq "EULEROS2.0SP9" and $arch eq "x86_64" and $kernel !~ /4.18.0-147.5.*$arch/) or
        ($DISTRO eq "EULEROS2.0SP9" and $arch eq "aarch64" and $kernel !~ /4.19.90-vhulk*.*\.$arch/) or
        ($DISTRO eq "OPENEULER20.03" and $kernel !~ /4.19.90-.*\.$arch/) or
        ($DISTRO =~ "RHEL8" and $kernel !~ /4.18.0-.*el8.*$arch/) or
        ($DISTRO eq "RHEL7.4ALTERNATE" and $kernel !~ /4.11.0-.*el7a.*$arch/) or
        ($DISTRO eq "RHEL7.5ALTERNATE" and $kernel !~ /4.14.0-.*el7a.*$arch/) or
        ($DISTRO eq "RHEL7.6ALTERNATE" and $kernel !~ /4.14.0-.*el7a.*$arch/) or
        ($DISTRO =~ /RHEL7\.\d+$/ and $kernel !~ /3.10.0-.*el7.*$arch/) or
        ($DISTRO =~ "RHEL6" and $kernel !~ /2.6.32-.*el6.*$arch/)) {
        $kmp = 0;
    }
    # SLES
    if (($rpm_distro eq "sles12sp1" and $kernel !~ /3.12.[1-9]+-[0-9].*/) or
        ($rpm_distro eq "sles12sp2" and $kernel !~ /4.4.[1-9]+-[0-9].*/) or
        ($rpm_distro eq "sles12sp3" and $kernel !~ /4.4.[0-9]+-[0-9].*/) or
        ($rpm_distro eq "sles12sp4" and $kernel !~ /4.12.[0-9]+-[0-9].*/) or
        ($rpm_distro eq "sles12sp0" and $kernel !~ /3.12.[1-9][2-9]-[0-9].*/) or
        ($rpm_distro eq "sles15sp0" and $kernel !~ /4.12.[1-9][2-9]-[0-9].*/) or
        ($rpm_distro eq "sles15sp1" and $kernel !~ /4.12.14-[0-9].*/)) {
        $kmp = 0;
    }
    # OpenSUSE
    if (($DISTRO eq "openSUSE12.1" and $kernel !~ /3\.1\.[0-9]/) or
        ($DISTRO eq "openSUSE_TUMBLEWEED" and $kernel !~ /4\.3\.0/)) {
        $kmp = 0;
    }
    # Oracle
    if (($DISTRO =~ /^(OL7\.[016-9] | OL6\.[1-6] | OL5\..*)$/x)
        or ($DISTRO eq "OL6.7" and $kernel !~ /3\.8\.13-68\..*el6uek/)
        or ($DISTRO eq "OL6.8" and $kernel !~ /4\.1\.12-37\..*el6uek/)
        or ($DISTRO eq "OL6.9" and $kernel !~ /4\.1\.12-61\..*el6uek/)
        or ($DISTRO eq "OL6.10" and $kernel !~ /4\.1\.12-124\..*el6uek/)
        or ($DISTRO eq "OL7.2" and $kernel !~ /3\.8\.13-92\..*el7uek/)
        or ($DISTRO eq "OL7.3" and $kernel !~ /4\.1\.12-61\..*el7uek/)
        or ($DISTRO eq "OL7.4" and $kernel !~ /4\.1\.12-94\..*el7uek/)
        or ($DISTRO eq "OL7.5" and $kernel !~ /4\.14\.35-1818\..*el7uek/)
       ) {
        $kmp = 0;
    }
    if (!$kmp) {
        print_and_log_colored("KMP is not supported on $DISTRO with non default kernel $kernel.", $verbose2, "RED");
        print_and_log_colored("Switching to non-KMP mode", $verbose2, "RED");
    }
}

# $speed_up_kmp is supported on RHEL only
if ($DISTRO !~ /RHEL/ or not $kmp) {
    print_and_log("WARNING: Speed up kmp is supported on RHEL only with in-box the kernel. Turning off...\n") if ($speed_up_kmp);
    $speed_up_kmp = 0;
}

# set which rpm to use for those rpms that support KMP
my $kernel_rpm;
my $knem_rpm = "knem";
my $knem_mods = "knem-modules";
my $xpmem_mods = "xpmem-modules";
my $kernel_mft_rpm;
my $en_kernel_rpm;

if ($kmp) {
    $kernel_rpm = "mlnx-ofa_kernel";
    $knem_mods = ""; # we have KMPs instead
    $xpmem_mods = "xpmem";
    $kernel_mft_rpm = "kernel-mft-mlnx";
    $en_kernel_rpm = "mlnx-en";
} else {
    $kernel_rpm = "mlnx-ofa_kernel-modules";
    $kernel_mft_rpm = "kernel-mft";
    $en_kernel_rpm = "mlnx_en";
    # disable --bump-kmp-version flag when KMP=0
    $kmp_build_num = "";
}

my $iser_mods = "iser";
my $isert_mods = "isert";
my $srp_mods = "srp";
my $mlnx_nfsrdma_mods = "mlnx-nfsrdma";
my $mlnx_nvme_mods = "mlnx-nvme";
my $virtiofs_mods = "virtiofs";

if ($build_dkms_packages) {
    $kernel_rpm = "mlnx-ofa_kernel-dkms";
    $knem_mods = "knem-dkms";
    $xpmem_mods = "xpmem-dkms";
    $kernel_mft_rpm = "kernel-mft-dkms";
    $iser_mods = "iser-dkms";
    $isert_mods = "isert-dkms";
    $srp_mods = "srp-dkms";
    $mlnx_nfsrdma_mods = "mlnx-nfsrdma-dkms";
    $mlnx_nvme_mods = "mlnx-nvme-dkms";
    $virtiofs_mods = "virtiofs-dkms";
}

my @kernel_packages = ($kernel_rpm, "mlnx-ofa_kernel-devel", "mlnx-ofa_kernel-source", $knem_rpm, $knem_mods, $xpmem_mods, $kernel_mft_rpm, $iser_mods, $srp_mods, $isert_mods, $mlnx_nfsrdma_mods, $mlnx_nvme_mods, $virtiofs_mods);
my @basic_kernel_modules = ("core", "mlxfw", "mlx5", "ipoib");
my @ulp_modules = ("sdp", "srp", "srpt", "rds", "iser", "e_ipoib", "nfsrdma", 'isert');

# kernel modules in "technology preview" status can be installed by
# adding "module=y" to the ofed.conf file in unattended installation mode
# or by selecting the module in custom installation mode during interactive installation
my @tech_preview;

my @kernel_modules = (@basic_kernel_modules, @ulp_modules);

my $kernel_configure_options_global = '';
my $user_configure_options = '';

my @misc_packages = ("ofed-docs", "ofed-scripts", "mlnx-tools");

my @mpi_packages = ( "mpi-selector",
                     "openmpi",
                    );

my @user_packages = ("libibverbs", "rdma-core-devel",
                     "libibverbs-utils",
                     "rdma-core-debuginfo", "libmlx5-rdmav2",
                     "libibumad",
                     "ibsim", "ibsim-debuginfo", "ibacm",
                     "librdmacm", "librdmacm-utils",
                     "opensm", "opensm-libs", "opensm-devel", "opensm-debuginfo", "opensm-static",
                     "dapl", "dapl-devel", "dapl-devel-static", "dapl-utils", "dapl-debuginfo",
                     "perftest", "multiperf", "multiperf-debuginfo", "mstflint",
                     "srp_daemon",
                     "ibutils2", "cc_mgr", "dump_pr", "ar_mgr",
                     "infiniband-diags", "libibnetdisc5",
                     "ofed-docs", "ofed-scripts", "dpcp", "dpcp-debuginfo", "mlnx-tools",
                     "ucx", "ucx-devel", "ucx-static",
                     "ucx-cma", "ucx-ib", "ucx-ib-mlx5", "ucx-rdmacm", "ucx-cuda", "ucx-gdrcopy", "ucx-knem", "ucx-xpmem",
                     @mpi_packages,
                     "libvma", "libvma-utils", "libvma-devel", "libvma-debuginfo",
                     "libxlio", "libxlio-utils", "libxlio-devel",
                     "sockperf",
                     "fabric-collector", "fabric-collector-debuginfo",
                     "mlnx-ethtool", "mlnx-ethtool-debuginfo",
                     "mlnx-iproute2",
                     "$openvswitch", "spdk",
                     "libpka", "mlxbf-bootctl", "mlxbf-bootimages", "bfscripts", "nvme-snap", "rshim",
                     "python3-pyverbs",
                     "xpmem", "libxpmem", "$xpmem_mods",
                     "clusterkit",
                     "ibarr",
                     );

my @mft_packages = (
                   "mft-int", "kernel-mft", "mft-compat", "mft-oem"
                   );

my @dpdk_packages = (
                   "mlnx-dpdk", "mlnx-dpdk-devel",
                   );

# List of packages that were included in the previous OFED releases
# for uninstall purpose
my @prev_ofed_packages = (
    "kernel-ib", "kernel-ib-devel", "mlnx-ofa_kernel", "mlnx-ofa_kernel-source",
    "mpich_mlx", "ibtsal", "openib",
    "opensm", "opensm-devel", "opensm-libs", "opensm-libs3",
    "opensm-debuginfo", "opensm-static",
    "libopensmssa", "libopensmssa-debuginfo", "ibssa", "ibssa-debuginfo",
    "ibacm_ssa", "ibacm_ssa-devel","ibacm_ssa-debuginfo",
    "mpi_ncsa", "mpi_osu", "thca", "ib-osm", "osm", "diags", "ibadm",
    "ib-diags", "ibgdiag", "ibdiag", "ib-management",
    "ib-verbs", "ib-ipoib", "ib-cm", "ib-sdp", "ib-dapl",
    "udapl", "udapl-devel", "libdat", "libibat", "ib-kdapl", "ib-srp", "ib-srp_target",
    "libehca", "libehca-devel-static", "libehca-debuginfo",
    "libibcm", "libibcm-devel", "libibcm-debuginfo",
    "ibvexdmtools", "libibcommon", "libibcommon-devel",
    "libibmad", "libibmad-devel", "libibmad-static", "libibmad-debuginfo",
    "libibumad", "libibumad-devel", "libibumad-static", "libibumad-debuginfo",
    "ibsim", "ibsim-debuginfo", "ibacm",
    "ar_mgr", "dump_pr",
    "hcoll", "hcoll-cuda",
    "ar_mgr", "dump_pr",
    "knem",
    "qperf",
    "sharp",
    "ucx", "ucx-cma", "ucx-cuda", "ucx-devel", "ucx-gdrcopy", "ucx-ib", "ucx-ib-cm", "ucx-ib-mlx5",
    "ucx-knem", "ucx-rdmacm",
    "libibprof",
    "libibverbs1", "libibverbs", "libibverbs-devel", "libibverbs-utils",
    "libibverbs-devel-static", "libibverbs-runtime", "libibverbs1-32bit",
    "libibverbs-debuginfo",
    "libipathverbs", "libipathverbs-devel", "libipathverbs-debuginfo",
    "libmthca", "libmthca-devel-static", "libmthca-debuginfo",
    "libmlx5", "libmlx5-devel", "libmlx5-debuginfo",
    "libmlx4", "libmlx4-devel", "libmlx4-debuginfo", "libmlx4-1", "libefa1",
    "mlx4_accl", "mlx4_accl_sys",
    "librdmacm1", "librdmacm", "librdmacm-utils", "librdmacm-devel", "librdmacm-debuginfo",
    "openib-diags", "openib-mstflint", "openib-perftest", "openib-srptools", "openib-tvflash",
    "openmpi", "openmpi-devel", "openmpi-libs",
    "openmpi-psm", "compat-openmpi", "compat-openmpi-psm", "mpitests-openmpi",
    @mpi_packages,
    "openshmem", "bupc", "fca",
    "ibutils", "ibutils-devel", "ibutils-libs",
    "ibutils2", "ibutils2-devel",
    "libnes", "libnes-devel", "libnes-devel-static", "libnes-debuginfo",
    "infinipath-psm", "infinipath-psm-devel",
    "mellanox-firmware", "mellanox-ofed", "mlnxofed",
    @mft_packages,
    "compat-dapl", "compat-dapl-devel", "compat-dapl-utils",
    "dapl", "dapl-devel", "dapl-devel-static", "dapl-utils", "dapl-debuginfo",
    "mvapich", "mvapich2",
    "mvapich_gcc", "openmpi_gcc", "mvapich2_gcc", "mpitests_mvapich2", "mpitests_openmpi",
    "fabric-collector", "fabric-collector-debuginfo",
    "libsdp", "libsdp-devel", "libsdp-debuginfo",
    "sdpnetstat-debuginfo", "sdpnetstat",
    "intel-mic-ofed-dapl-utils",
    "mlnx-ethtool", "mlnx-ethtool-debuginfo",
    "mlnx-iproute2",
    "libdisni", "libdisni-devel", "libdisni-debuginfo",
    @openvswitch_packages, "spdk",
    "libpka", "mlxbf-bootctl", "mlxbf-bootimages", "bfscripts", "nvme-snap", "rshim",
    "perftest", "multiperf", "multiperf-debuginfo",
    "libusnic_verbs",
    "mlnx-fw-updater",
    "srp_daemon",
    "libmlx5-1",
    "mxm",
    "mellanox-mlnxen",
    "libiwpm",
    "infiniband-diags", "infiniband-diags-compat", "libibnetdisc5",
    "infiniband-diags-devel", "infiniband-guest",
    "mlx-steering-dump",
    "libmlx5-rdmav2",
    "librxe", "librxe-devel-static", "librxe-debuginfo",
    "libmverbs", "libmverbs-devel", , "libmqe", "libmqe-devel",
    "libcxgb3", "libcxgb3-devel", "libcxgb3-debuginfo",
    "libcxgb4", "libcxgb4-devel", "libcxgb4-debuginfo",
    "compat-opensm-libs", "opensm-libs3",
    "mstflint",
    "qlvnictools", "srptools", "rds-tools", "rds-devel", "rnfs-utils",
    "ofed-docs", "ofed-scripts", "mlnx-tools",
    "libamso-rdmav2", "libamso-rdmav2-devel",
    "hp-mlnx_ofed", "mellanox-mlnx_en",
    "libvma", "libvma-utils", "libvma-devel", "libvma-debuginfo",
    "sockperf",
    "ibarr",
    "kmod-mellanox", "kmod-mellanox-ethernet", "kmod-mellanox-nvme",
    );

my @basic_kernel_packages = ($kernel_rpm, "mlnx-ofa_kernel-devel", $kernel_mft_rpm, $iser_mods, $srp_mods, $isert_mods, $mlnx_nfsrdma_mods, $mlnx_nvme_mods, $virtiofs_mods);
my @basic_user_packages = ("libibverbs", "libibverbs-utils",
                            "librdmacm", "librdmacm-utils",
                            "mstflint", @misc_packages, "mlnx-ethtool", "mlnx-iproute2", "python3-pyverbs");

my @bluefield_kernel_packages = (@basic_kernel_packages, $knem_rpm, $knem_mods);

my @bluefield_kernel_modules = ("core", "mlxfw", "mlx5", "ipoib", "iser", "isert");
my @bluefield_user_packages = ("libibverbs", "rdma-core-devel", "libibverbs-utils", "librdmacm", "librdmacm-utils", "ibacm", "perftest","mstflint", "infiniband-diags", "opensm", "opensm-libs", "opensm-devel", "opensm-static", "mft", "ibutils2", "mlnx-ethtool", "mlnx-iproute2", "openvswitch", "libpka", "mlxbf-bootctl", "mlxbf-bootimages", "bfscripts", "nvme-snap", "spdk", "ucx", "ucx-devel", "ucx-static", "ucx-cma", "ucx-ib", "ucx-ib-mlx5", "ucx-knem", "ucx-rdmacm", "rshim");

my @hpc_kernel_packages = (@basic_kernel_packages, $knem_rpm, $knem_mods);
my @hpc_kernel_modules = (@basic_kernel_modules);
my @hpc_user_packages = (@basic_user_packages, "ucx", "ucx-devel", "ucx-static", ,"ibacm", "librdmacm",
                        "ucx-cma", "ucx-ib", "ucx-ib-mlx5", "ucx-knem", "ucx-rdmacm", "ucx-xpmem",
                        "librdmacm-utils", "dapl", "dapl-devel", "dapl-devel-static", "dapl-utils",
                        "infiniband-diags", "ibutils2", "cc_mgr", "dump_pr", "ar_mgr", "mstflint", "perftest", "multiperf", @mpi_packages);

my @vma_kernel_packages = (@basic_kernel_packages);
my @vma_kernel_modules = (@basic_kernel_modules);
my @vmavpi_user_packages = ("libibverbs", "rdma-core-devel", "libibverbs-utils",
                            "ibacm", "librdmacm", "librdmacm-utils", "perftest", "multiperf", "mstflint", "infiniband-diags",
                            "opensm", "opensm-libs", "opensm-devel", "opensm-static", "mft", "ibutils2", @misc_packages, "libvma", "libvma-utils", "libvma-devel", "sockperf", "mlnx-ethtool",
                            "mlnx-iproute2", "dpcp", "rshim",
                            );
my @vma_user_packages = (@vmavpi_user_packages);
my @vmaeth_user_packages = ("libibverbs", "rdma-core-devel", "libibverbs-utils",
                            "ibacm", "librdmacm", "librdmacm-utils", "perftest", "multiperf", "mstflint", "mft", "ibutils2",
                            "rshim",
                            @misc_packages, "libvma", "libvma-utils", "libvma-devel", "sockperf", "mlnx-ethtool", "mlnx-iproute2", "dpcp");

my @xlio_kernel_packages = (@basic_kernel_packages);
my @xlio_kernel_modules = (@basic_kernel_modules);
my @xlio_user_packages = ("libibverbs", "rdma-core-devel", "libibverbs-utils",
                            "ibacm", "librdmacm", "librdmacm-utils", "perftest", "multiperf", "mstflint", "infiniband-diags",
                            "opensm", "opensm-libs", "opensm-devel", "opensm-static", "mft", "ibutils2", @misc_packages, "sockperf", "mlnx-ethtool",
                            "mlnx-iproute2", "dpcp", "libxlio", "libxlio-utils", "libxlio-devel", "rshim",
                            );
my @hypervisor_kernel_packages = (@basic_kernel_packages);
my @guest_kernel_packages = (@basic_kernel_packages);
my @guest_kernel_modules = ("core","mlxfw","mlx5","ipoib","srp","iser", 'isert', 'mlnx-nfsrdma');
my @hypervisor_kernel_modules = ("core","mlxfw","mlx4_vnic","mlx5","ipoib","srp","iser", 'isert', 'mlnx-nfsrdma');

my @sroiv_common_user_packages = ("udapl","ofed-scripts",
					"ibacm","librdmacm","librdmacm-utils",
					"libibverbs","rdma-core-devel","libibverbs-utils",
					"libibumad",
					"perftest", "multiperf",
					"mlnx-tools",
					"srp_daemon", "mlnx-ethtool");

my @hypervisor_user_packages = (@sroiv_common_user_packages,"infiniband-diags","osm","mstflint","ofed-docs",
					"dapl","dapl-devel","dapl-devel-static","dapl-utils",
					"opensm","opensm-libs","opensm-devel","opensm-static","ibutils2",
					"ibarr",
					);

my @guest_user_packages = (@sroiv_common_user_packages,"infiniband-diags", @mpi_packages);

my @eth_kernel_packages = ($en_kernel_rpm, "mlnx-en-utils", "mlnx-en-sources", "mlnx-en-doc");
my @eth_kernel_modules = ("core", "mlxfw", "mlx5");
my @eth_user_packages = ("mstflint");

my @dpdk_kernel_packages = ($kernel_rpm, "mlnx-ofa_kernel-devel");
my @dpdk_kernel_modules = (@kernel_modules);
my @dpdk_user_packages = ("libibverbs", "rdma-core-devel", "libibverbs-utils", "librdmacm", "librdmacm-utils", "ibacm", "mstflint", "mlnx-ethtool", "mlnx-iproute2");

# all_packages is required to save ordered (following dependencies) list of
# packages. Hash does not saves the order
my @all_packages = (@kernel_packages, @user_packages);

# which modules are required for the standalone module rpms
my %standalone_kernel_modules_info = (
        'iser' => ["core", "ipoib"],
        'isert' => ["core", "ipoib"],
        'srp' => ["core", "ipoib"],
        'mlnx-nfsrdma' => ["core"],
        'mlnx-nvme' => ["core"],
        'mlnx-rdma-rxe' => ["core"],
        'fwctl' => ["core"],
        'virtiofs' => ["core"],
);

my @ucx_dist_build_req = (
    "$pkgconfig", "$libstdc_devel", "$gcc_cpp", "binutils-devel", "$libstdc"
);

my %kernel_modules_info = (
        'core' =>
            { name => "core", available => 1, selected => 0,
            included_in_rpm => 0, requires => [], },
        'mlxfw' =>
            { name => "mlxfw", available => 1, selected => 0,
            included_in_rpm => 0, requires => [], },
        'mthca' =>
            { name => "mthca", available => 0, selected => 0,
            included_in_rpm => 0, requires => ["core"], },
        'mlx4' =>
            { name => "mlx4", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core"], },
        'mlx5' =>
            { name => "mlx5", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core"], },
        'mlx4_en' =>
            { name => "mlx4_en", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core","mlx4"], },
        'mlx4_vnic' =>
            { name => "mlx4_vnic", available => 0, selected => 0,
            included_in_rpm => 0, requires => ["core","mlx4"], },
        'mlx4_fc' =>
            { name => "mlx4_fc", available => 0, selected => 0,
            included_in_rpm => 0, requires => ["core","mlx4_en"], },
        'ipoib' =>
            { name => "ipoib", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core"], },
        'sdp' =>
            { name => "sdp", available => 0, selected => 0,
            included_in_rpm => 0, requires => ["core", "ipoib"], },
        'srp' =>
            { name => "srp", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core", "ipoib"], },
        'srpt' =>
            { name => "srpt", available => 0, selected => 0,
            included_in_rpm => 0, requires => ["core"], },
        'rds' =>
            { name => "rds", available => 0, selected => 0,
            included_in_rpm => 0, requires => ["core", "ipoib"], },
        'e_ipoib' =>
            { name => "e_ipoib", available => 0, selected => 0,
            included_in_rpm => 0, requires => ["core", "ipoib"], },
        'iser' =>
            { name => "iser", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core", "ipoib"], ofa_req_inst => [] },
        'isert' =>
            { name => "isert", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core", "ipoib"], ofa_req_inst => [] },
        'nfsrdma' =>
            { name => "nfsrdma", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core", "ipoib"], },
        'mlx5_fpga_tools' =>
            { name => "mlx5_fpga_tools", available => 1, selected => 0,
            included_in_rpm => 0, requires => ["core"], },
        );

my @kernel_build_deps = ("make", "$gcc", "patch", "autoconf", "automake", @kernel_req, "$gcc_toolset_11");
my %packages_info = (
        # Kernel packages
        'mlnx-ofa_kernel' =>
           { name => "mlnx-ofa_kernel", parent => "mlnx-ofa_kernel",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1, mode => "kernel",
           dist_req_build => [@kernel_build_deps],
           dist_req_inst => ["ethtool", "lsof", "pciutils", "$srvtool"],
           ofa_req_build => [],
           ofa_req_inst => ["ofed-scripts", "mlnx-tools"],
           soft_req => ["ofed-scripts"],
           configure_options => '' },
        'mlnx-ofa_kernel-devel' =>
           { name => "mlnx-ofa_kernel-devel", parent => "mlnx-ofa_kernel",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1, mode => "kernel", dist_req_build => [],
           dist_req_inst => ["make"], ofa_req_build => [], ofa_req_inst => ["ofed-scripts", 'mlnx-ofa_kernel'], configure_options => '' },
        'mlnx-ofa_kernel-source' =>
           { name => "mlnx-ofa_kernel-devel", parent => "mlnx-ofa_kernel",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1,
           mode => "kernel",
           dist_req_build => [@kernel_build_deps],
           dist_req_inst => [],
           ofa_req_build => [],
           ofa_req_inst => [],
           },
        'mlnx-ofa_kernel-dkms' =>
           { name => "mlnx-ofa_kernel-dkms", parent => "mlnx-ofa_kernel",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1,
           mode => "kernel",
           dist_req_build => [],
           dist_req_inst => [],
           ofa_req_build => [],
           ofa_req_inst => ["mlnx-ofa_kernel-source", "ofed-scripts", 'mlnx-ofa_kernel'],
           },
        'mlnx-ofa_kernel-modules' =>
            { name => "mlnx-ofa_kernel-modules", parent => "mlnx-ofa_kernel",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => [@kernel_build_deps],
            dist_req_inst => ["lsof","pciutils"],
            ofa_req_build => [], ofa_req_inst => ["mlnx-ofa_kernel", "ofed-scripts"],
            soft_req => ["ofed-scripts"] },
        'mlnx-tools' =>
            { name => "mlnx-tools", parent => "mlnx-tools",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => ["$python_rpm_macros", "$python_unrevisioned"],
            dist_req_inst => ["$python"],
            ofa_req_build => [],
            ofa_req_inst => []
            },
        'kernel-mft' =>
            { name => "kernel-mft", parent => "kernel-mft",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", $gcc_toolset_11],
            dist_req_inst => [], ofa_req_build => [], ofa_req_inst => ["ofed-scripts"], configure_options => '' },
        'kernel-mft-mlnx' =>
            { name => "kernel-mft-mlnx", parent => "kernel-mft",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", $gcc_toolset_11],
            dist_req_inst => [], ofa_req_build => [], ofa_req_inst => ["ofed-scripts"], configure_options => '' },
        'kernel-mft-dkms' =>
            { name => "kernel-mft-dkms", parent => "kernel-mft",
            available => 1, mode => "user",
            dist_req_build => ["make", "$gcc", "libtool"],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => [],
            },
        'knem' =>
            { name => "knem", parent => "knem",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", "libtool" , @kernel_req,
                $gcc_toolset_11],
            dist_req_inst => [], ofa_req_build => [], ofa_req_inst => [], configure_options => '' },
        # this one is used when KMP is disabled to proivde the kernel modules only
        'knem-modules' =>
            { name => "knem-modules", parent => "knem",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", "libtool" , @kernel_req,
                $gcc_toolset_11],
            dist_req_inst => [],
            ofa_req_build => ["knem"],
            ofa_req_inst => ["knem"],
            configure_options => '' },
        'knem-dkms' =>
            { name => "knem-dkms", parent => "knem",
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", "libtool"],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ['knem'],
            },
        'xpmem' =>
            { name => "xpmem", parent => "xpmem",
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", "libtool" , @kernel_req,
                $gcc_toolset_11],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => [],
            },
        'xpmem-modules' =>
            { name => "xpmem-modules", parent => "xpmem",
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", "libtool" , @kernel_req,
                $gcc_toolset_11],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ['xpmem'],
            },
        'xpmem-dkms' =>
            { name => "xpmem-dkms", parent => "xpmem",
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", "libtool"],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ['xpmem'],
            },
        'libxpmem' =>
            { name => "libxpmem", parent => "xpmem",
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", "libtool" , @kernel_req,
                $gcc_toolset_11],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => [],
            },
        'libxpmem-devel' =>
            { name => "libxpmem-devel", parent => "xpmem",
            available => 1, mode => "kernel",
            dist_req_build => ["make", "$gcc", "libtool" , @kernel_req,
                $gcc_toolset_11],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ['libxpmem'],
            },

        'mlnx-en' =>
           { name => "mlnx-en", parent => "mlnx-en",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1, mode => "kernel",
           dist_req_build => [@kernel_build_deps],
           dist_req_inst => ["ethtool", "lsof", "pciutils"],
           ofa_req_build => [],
           ofa_req_inst => ['mlnx-en-utils'],
           configure_options => '' },
        'mlnx_en' =>
           { name => "mlnx_en", parent => "mlnx-en",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1, mode => "kernel",
           dist_req_build => [@kernel_build_deps],
           dist_req_inst => ["ethtool", "lsof", "pciutils"],
           ofa_req_build => [],
           ofa_req_inst => ['mlnx-en-utils'],
           configure_options => '' },
        'mlnx-en-utils' =>
           { name => "mlnx-en-utils", parent => "mlnx-en",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1, mode => "kernel",
           dist_req_build => [@kernel_build_deps],
           dist_req_inst => [],
           ofa_req_build => [],
           ofa_req_inst => ['ofed-scripts', "mlnx-tools"],
           configure_options => '' },
        'mlnx-en-sources' =>
           { name => "mlnx-en-sources", parent => "mlnx-en",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1, mode => "kernel",
           dist_req_build => [@kernel_build_deps],
           dist_req_inst => [],
           ofa_req_build => [],
           ofa_req_inst => [],
           configure_options => '' },
        'mlnx-en-doc' =>
           { name => "mlnx-en-doc", parent => "mlnx-en",
           selected => 0, installed => 0, rpm_exist => 0,
           available => 1, mode => "kernel",
           dist_req_build => [],
           dist_req_inst => [],
           ofa_req_build => [],
           ofa_req_inst => [],
           configure_options => '' },

        # User space libraries
        'mlx-steering-dump' =>
            { name => "mlx-steering-dump", parent => "mlx-steering-dump",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$python"],
            dist_req_inst => ["$python"], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0 },
        'libibverbs' =>
            { name => "libibverbs", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => ["$pkgconfig","$valgrind_devel", "$gcc", "$glibc_devel","$libgcc", "$libnl_devel"],
            dist_req_inst => ["$pkgconfig","$libnl"] ,
            ofa_req_build => [],
            ofa_req_inst => ["ofed-scripts", "rdma-core"],
            exception => 0, configure_options => '' },
        'libibverbs-utils' =>
            { name => "libibverbs-utils", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => [],
            ofa_req_inst => ["libibverbs"],
            exception => 0 },

        # Management
        'libibumad' =>
            { name => "libibumad", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["libtool"],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ["rdma-core"],
            exception => 0, configure_options => '' },

        'opensm' =>
            { name => "opensm", parent => "opensm",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["bison", "flex", "$gcc_cpp"],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["opensm-libs", "libibumad"],
            exception => 0, configure_options => '' },
        'opensm-devel' =>
            { name => "opensm-devel", parent => "opensm",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["rdma-core-devel", "opensm-libs"],
            exception => 0 },
        'opensm-libs' =>
            { name => "opensm-libs", parent => "opensm",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["bison", "flex"],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["libibumad"],
            exception => 0 },
        'opensm-static' =>
            { name => "opensm-static", parent => "opensm",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["rdma-core-devel", "opensm-libs"],
            exception => 0 },
        'opensm-debuginfo' =>
            { name => "opensm-debuginfo", parent => "opensm",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => [],
            exception => 0 },

        'ibsim' =>
            { name => "ibsim", parent => "ibsim",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$gcc_toolset_11_annobin"],
            dist_req_inst => [], ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["libibumad", "infiniband-diags"],
            exception => 0, configure_options => '' },
        'ibsim-debuginfo' =>
            { name => "ibsim-debuginfo", parent => "ibsim",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => [],
            exception => 0, configure_options => '' },

        'ibacm' =>
            { name => "ibacm", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel", "mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["libibverbs", "libibumad", "rdma-core"],
            exception => 0, configure_options => '' },
        'librdmacm' =>
            { name => "librdmacm", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$valgrind_devel"],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["libibverbs", "rdma-core"],
            exception => 0, configure_options => '' },
        'librdmacm-utils' =>
            { name => "librdmacm-utils", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["librdmacm", "rdma-core"],
            exception => 0 },
        'librdmacm-debuginfo' =>
            { name => "librdmacm-debuginfo", parent => "librdmacm",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => [],
            exception => 0 },

        'perftest' =>
            { name => "perftest", parent => "perftest",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["pciutils-devel"],
            dist_req_inst => [], ofa_req_build => ["rdma-core-devel", "librdmacm"],
            ofa_req_inst => ["libibverbs", "librdmacm", "libibumad"],
            exception => 0, configure_options => '' },
        'perftest-debuginfo' =>
            { name => "perftest-debuginfo", parent => "perftest",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0 },

        'mft' =>
            { name => "mft", parent => "mft",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$libexpat", "$libexpat_devel", "tcl", "$tcl_devel", "$tk", $libstdc_devel ],
            dist_req_inst => ["$libexpat", "tcl", "$tk", $libstdc],
            ofa_req_build => [], ofa_req_inst => [], configure_options => '' },
        'mft-debuginfo' =>
            { name => "mft-debuginfo", parent => "mft",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => [], ofa_req_inst => [], },

        'mstflint' =>
            { name => "mstflint", parent => "mstflint",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => ["$zlib_devel", "$libstdc_devel", "$gcc_cpp", "$openssl_devel"],
            dist_req_inst => ["$openssl"], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0, configure_options => '' },
        'mstflint-debuginfo' =>
            { name => "mstflint-debuginfo", parent => "mstflint",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0 },

        'ibutils2' =>
            { name => "ibutils2", parent => "ibutils2",
            selected => 0, installed => 0, rpm_exist => 0, internal => 1,
            available => 1, mode => "user", dist_req_build => ["tcl", "$tcl_devel", "$libstdc_devel", "$gcc_cpp"],
            dist_req_inst => ["tcl", "$libstdc"], ofa_req_build => ["rdma-core-devel", "infiniband-diags"],
            ofa_req_inst => ["libibumad", "infiniband-diags"],
            exception => 0, configure_options => '' },
        'ibutils2-debuginfo' =>
            { name => "ibutils2-debuginfo", parent => "ibutils2",
            selected => 0, installed => 0, rpm_exist => 0, internal => 1,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0 },

        'ar_mgr' =>
            { name => "ar_mgr", parent => "ar_mgr",
            selected => 0, installed => 0, rpm_exist => 0, internal => 1,
            available => 1, mode => "user", dist_req_build => ["$libstdc_devel"],
            dist_req_inst => ["$libstdc"], ofa_req_build => ["opensm-libs", "opensm-devel", "ibutils2"],
            ofa_req_inst => ["opensm", "ibutils2"],
            exception => 0, configure_options => '' },

        'cc_mgr' =>
            { name => "cc_mgr", parent => "cc_mgr",
            selected => 0, installed => 0, rpm_exist => 0, internal => 1,
            available => 1, mode => "user", dist_req_build => ["$libstdc_devel"],
            dist_req_inst => ["$libstdc"], ofa_req_build => ["opensm-libs", "opensm-devel", "ibutils2"],
            ofa_req_inst => ["opensm", "ibutils2"],
            exception => 0, configure_options => '' },

        'dump_pr' =>
            { name => "dump_pr", parent => "dump_pr",
            selected => 0, installed => 0, rpm_exist => 0, internal => 1,
            available => 1, mode => "user", dist_req_build => ["$libstdc_devel"],
            dist_req_inst => ["$libstdc"], ofa_req_build => ["opensm-libs", "opensm-devel"],
            ofa_req_inst => ["opensm"],
            exception => 0, configure_options => '' },

        'ibdump' =>
            { name => "ibdump", parent => "ibdump",
            selected => 0, installed => 0, rpm_exist => 0, internal => 1,
            available => 1, mode => "user", dist_req_build => ["$libstdc_devel", "$libcrypt"],
            dist_req_inst => ["$libstdc"], ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["libibverbs"],
            exception => 0, configure_options => '' },
        'ibdump-debuginfo' =>
            { name => "ibdump-debuginfo", parent => "ibdump",
            selected => 0, installed => 0, rpm_exist => 0, internal => 1,
            available => 0, mode => "user", dist_req_build => [],
            dist_req_inst => ['ibdump'], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0 },

        'infiniband-diags' =>
            { name => "infiniband-diags", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$glib2_devel", "$python_docutils"],
            dist_req_inst => ["$glib2"], ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["libibumad"],
            exception => 0, configure_options => '' },
        'infiniband-diags-guest' =>
            { name => "infiniband-diags-guest", parent => "infiniband-diags",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$glib2_devel"],
            dist_req_inst => ["$glib2"], ofa_req_build => [],
            ofa_req_inst => ["infiniband-diags"],
            exception => 0 },
        'infiniband-diags-compat' =>
            { name => "infiniband-diags-compat", parent => "infiniband-diags",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$glib2_devel"],
            dist_req_inst => ["$glib2"], ofa_req_build => [],
            ofa_req_inst => ["infiniband-diags"],
            exception => 0 },
        'infiniband-diags-debuginfo' =>
            { name => "infiniband-diags-debuginfo", parent => "infiniband-diags",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$glib2_devel"],
            dist_req_inst => ["$glib2"], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0 },

        'dapl' =>
            { name => "dapl", parent => "dapl",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["libibverbs", "rdma-core-devel", "librdmacm"],
            ofa_req_inst => ["libibverbs", "librdmacm"],
            exception => 0, configure_options => '' },
        'dapl-devel' =>
            { name => "dapl-devel", parent => "dapl",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["libibverbs","rdma-core-devel", "librdmacm"],
            ofa_req_inst => ["dapl"],
            exception => 0, configure_options => '' },
        'dapl-devel-static' =>
            { name => "dapl-devel-static", parent => "dapl",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["libibverbs","rdma-core-devel", "librdmacm"],
            ofa_req_inst => ["dapl"],
            exception => 0, configure_options => '' },
        'dapl-utils' =>
            { name => "dapl-utils", parent => "dapl",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["libibverbs","rdma-core-devel", "librdmacm"],
            ofa_req_inst => ["dapl"],
            exception => 0, configure_options => '' },
        'dapl-debuginfo' =>
            { name => "dapl-debuginfo", parent => "dapl",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => ["libibverbs","rdma-core-devel", "librdmacm"],
            ofa_req_inst => [],
            exception => 0 },

        'ucx' =>
            { name => "ucx", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => ["$pkgconfig","$libstdc"],
            ofa_req_build => ["rdma-core-devel", "knem", "libxpmem-devel"],
            ofa_req_inst => [],
            soft_req => ["knem", "libxpmem-devel"],
            exception => 0, configure_options => '' },
        'ucx-devel' =>
            { name => "ucx-devel", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => ["$pkgconfig","$libstdc"],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ['ucx'],
            exception => 0, configure_options => '' },
        'ucx-static' =>
            { name => "ucx-static", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 0, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => ["$pkgconfig","$libstdc"],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ['ucx'],
            exception => 0, configure_options => '' },
        'ucx-cma' =>
            { name => "ucx-cma", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ['ucx'],
            exception => 0, configure_options => '' },
        'ucx-cuda' =>
            { name => "ucx-cuda", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 0, mode => "user",
            dist_req_build => [
                @ucx_dist_build_req, "cuda-libraries-$cuda_version",
                "cuda-cudart-devel-$cuda_version",
                "$nvidia_driver_devel", "cuda-nvcc-$cuda_version",
            ],
            dist_req_inst => ["cuda-cudart-$cuda_version"],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ['ucx'],
            exception => 0, configure_options => '' },
        'ucx-gdrcopy' =>
            { name => "ucx-gdrcopy", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 0, mode => "user",
            dist_req_build => [@ucx_dist_build_req, 'gdrcopy', 'gdrcopy-devel'],
            dist_req_inst => ['gdrcopy'],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ['ucx'],
            exception => 0, configure_options => '' },
        'ucx-ib' =>
            { name => "ucx-ib", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ['ucx', 'libibverbs'],
            exception => 0, configure_options => '' },
        'ucx-ib-mlx5' =>
            { name => "ucx-ib-mlx5", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ['ucx', 'ucx-ib', 'libibverbs'],
            exception => 0, configure_options => '' },
        'ucx-knem' =>
            { name => "ucx-knem", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel", 'knem'],
            ofa_req_inst => ['ucx', 'knem'],
            exception => 0, configure_options => '' },
        'ucx-rdmacm' =>
            { name => "ucx-rdmacm", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel", 'librdmacm'],
            ofa_req_inst => ['ucx', 'librdmacm'],
            exception => 0, configure_options => '' },
        'ucx-xpmem' =>
            { name => "ucx-xpmem", parent => "ucx",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [@ucx_dist_build_req],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core-devel", 'libxpmem-devel'],
            ofa_req_inst => ['ucx', 'libxpmem'],
            exception => 0,},

        'mpi-selector' =>
            { name => "mpi-selector", parent => "mpi-selector",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["tcsh"],
            dist_req_inst => ["tcsh"], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0, configure_options => '' },

        'openmpi' =>
            { name => "openmpi", parent => "openmpi",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$libgfortran","$libstdc_devel"],
            dist_req_inst => ["$libstdc"],
            ofa_req_build => ["rdma-core-devel", "ucx", "ucx-devel", "ucx-static", "$knem_rpm", "$knem_mods"],
            ofa_req_inst => ["libibverbs", "librdmacm", "mpi-selector", "ucx", "$knem_rpm", "$knem_mods"],
            soft_req => ["mpi-selector", "ucx-static", "$knem_rpm", "$knem_mods"],
            exception => 0, configure_options => '' },

        'clusterkit' =>
            { name => "clusterkit", parent => "clusterkit",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 0, mode => "user",
            dist_req_build => [
                "$fortran", "$curl_devel",
                "cuda-cudart-devel-$cuda_version", "cuda-nvcc-$cuda_version"
            ],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ["openmpi"],
            exception => 0, configure_options => '' },

        'ofed-docs' =>
            { name => "ofed-docs", parent => "ofed-docs",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => [],
            dist_req_inst => [], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0 },

        'ofed-scripts' =>
            { name => "ofed-scripts", parent => "ofed-scripts",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user", dist_req_build => ["$python_devel"],
            dist_req_inst => [], ofa_req_build => [],
            ofa_req_inst => [],
            exception => 0 },
        'libvma' =>
            { name => "libvma", parent=> "libvma",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["$libnl_devel", "$systemd_rpm_macros", "libtool"],
             dist_req_inst => ["$libnl"],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["libibverbs", "librdmacm"],
             exception => 0 },
        'libvma-utils' =>
            { name => "libvma-utils", parent=> "libvma",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["$libnl_devel", "$systemd_rpm_macros"],
             dist_req_inst => ["$libnl"],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["libibverbs", "librdmacm", "libvma"],
             exception => 0 },
        'libvma-devel' =>
            { name => "libvma-devel", parent=> "libvma",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["$libnl_devel", "$systemd_rpm_macros"],
             dist_req_inst => ["$libnl"],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["libibverbs", "librdmacm", "libvma"],
             exception => 0 },

        'libxlio' =>
            { name => "libxlio", parent=> "libxlio",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["$libnl_devel"],
             dist_req_inst => ["$libnl"],
             ofa_req_build => ["rdma-core-devel", "dpcp"],
             ofa_req_inst => ["libibverbs", "librdmacm", "dpcp"],
             soft_req => ["dpcp"],
             exception => 0 },
        'libxlio-utils' =>
            { name => "libxlio-utils", parent=> "libxlio",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["$libnl_devel"],
             dist_req_inst => ["$libnl"],
             ofa_req_build => ["rdma-core-devel", "dpcp"],
             ofa_req_inst => ["libibverbs", "librdmacm", "libxlio"],
             soft_req => ["dpcp"],
             exception => 0 },
        'libxlio-devel' =>
            { name => "libxlio-devel", parent=> "libxlio",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["$libnl_devel"],
             dist_req_inst => ["$libnl"],
             ofa_req_build => ["rdma-core-devel", "dpcp"],
             ofa_req_inst => ["libibverbs", "librdmacm", "libxlio", "dpcp"],
             soft_req => ["dpcp"],
             exception => 0 },

        'dpcp' =>
            { name => "dpcp", parent=> "dpcp",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["$pkgconfig", "automake", "autoconf", "libtool", "$gcc_cpp"],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["libibverbs"],
             exception => 0 },
        'dpcp-debuginfo' =>
            { name => "dpcp-debuginfo", parent=> "dpcp",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => [],
             exception => 0 },

        'sockperf' =>
            { name => "sockperf", parent=> "sockperf",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_build => ["doxygen"],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => [], exception => 0 },

        'fabric-collector' =>
            { name => "fabric-collector", parent=> "fabric-collector",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => ["$libcurl"],
             dist_req_build => ["$glibc_devel", "$curl_devel", "$openssl_devel"],
             ofa_req_build => ["rdma-core-devel", "opensm-devel"],
             ofa_req_inst => ["infiniband-diags", "libibumad", "opensm-libs"], exception => 0 },
        'fabric-collector-debuginfo' =>
            { name => "fabric-collector-debuginfo", parent=> "fabric-collector",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => ["$libcurl"],
             dist_req_build => ["$glibc_devel", "$curl_devel", "$openssl_devel"],
             ofa_req_build => ["rdma-core-devel", "opensm-devel"],
             ofa_req_inst => ["infiniband-diags", "libibumad", "opensm-libs"], exception => 0 },

        'iser' =>
            { name => "iser", parent => "iser",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel", dist_req_build => ["make", "$gcc"],
            dist_req_inst => ["pciutils","$python"],
            ofa_req_build => ["mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["ofed-scripts","$kernel_rpm"], configure_options => '' },

        'iser-dkms' =>
            { name => "iser-dkms", parent => "iser",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ["mlnx-ofa_kernel-dkms"],
            },

        'isert' =>
            { name => "isert", parent => "isert",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel", dist_req_build => ["make", "$gcc"],
            dist_req_inst => ["pciutils","$python"],
            ofa_req_build => ["mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["ofed-scripts","$kernel_rpm"], configure_options => '' },

        'isert-dkms' =>
            { name => "isert-dkms", parent => "isert",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ["mlnx-ofa_kernel-dkms"],
            },

        'srp' =>
            { name => "srp", parent => "srp",
            available => 1, mode => "kernel", dist_req_build => ["make", "$gcc"],
            dist_req_inst => ["pciutils","$python"],
            ofa_req_build => ["mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["ofed-scripts","$kernel_rpm"], configure_options => '' },

        'srp-dkms' =>
            { name => "srp-dkms", parent => "srp",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ["mlnx-ofa_kernel-dkms"],
            },

        'mlnx-nfsrdma' =>
            { name => "mlnx-nfsrdma", parent => "mlnx-nfsrdma",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel", dist_req_build => ["make", "$gcc"],
            dist_req_inst => ["pciutils","$python"],
            ofa_req_build => ["mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["ofed-scripts","$kernel_rpm"], configure_options => '' },

        'mlnx-nfsrdma-dkms' =>
            { name => "mlnx-nfsrdma-dkms", parent => "mlnx-nfsrdma",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ["mlnx-ofa_kernel-dkms"],
            },

        'mlnx-nvme' =>
            { name => "mlnx-nvme", parent => "mlnx-nvme",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel", dist_req_build => ["make", "$gcc"],
            dist_req_inst => ["pciutils","$python"],
            ofa_req_build => ["mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["ofed-scripts","$kernel_rpm"], configure_options => '' },

        'mlnx-nvme-dkms' =>
            { name => "mlnx-nvme-dkms", parent => "mlnx-nvme",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => [],
            ofa_req_inst => ["mlnx-ofa_kernel-dkms"],
            },

        'fwctl' =>
            { name => "fwctl", parent => "fwctl",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel", dist_req_build => ["make", "$gcc"],
            dist_req_inst => ["pciutils","$python"],
            ofa_req_build => ["mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["ofed-scripts","$kernel_rpm"], configure_options => '' },

        'virtiofs' =>
            { name => "virtiofs", parent => "virtiofs",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel", dist_req_build => ["make", "$gcc"],
            dist_req_inst => ["pciutils","$python"],
            ofa_req_build => ["mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["ofed-scripts","$kernel_rpm"], configure_options => '' },

        'virtiofs-dkms' =>
            { name => "virtiofs-dkms", parent => "virtiofs",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => ["mlnx-ofa_kernel-dkms"],
            ofa_req_inst => [],
            configure_options => '' },

        'mlnx-rdma-rxe' =>
            { name => "mlnx-rdma-rxe", parent => "mlnx-rdma-rxe",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "kernel", dist_req_build => ["make", "$gcc"],
            dist_req_inst => [],
            ofa_req_build => ["mlnx-ofa_kernel-devel"],
            ofa_req_inst => ["ofed-scripts","$kernel_rpm"], configure_options => '' },

        'libpka' =>
            { name => "libpka", parent=> "libpka",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["$openssl_devel"],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => [], exception => 0 },

        'mlxbf-bootctl' =>
            { name => "mlxbf-bootctl", parent=> "mlxbf-bootctl",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => [], exception => 0 },

        'mlxbf-bootimages' =>
            { name => "mlxbf-bootimages", parent=> "mlxbf-bootimages",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => [], exception => 0 },

        'bfscripts' =>
            { name => "bfscripts", parent=> "bfscripts",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => ["mlxbf-bootctl", "mlxbf-bootimages"], exception => 0 },

        'rshim' =>
            { name => "rshim", parent=> "rshim",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => ["$pciutils_libs", "$libusb", "$fuse_libs", "$kernel_modules_extra"],
             dist_req_build => ["pciutils-devel", "$libusb_devel", "fuse-devel"],
             ofa_req_build => [],
             ofa_req_inst => [], exception => 0 },

        'nvme-snap' =>
            { name => "nvme-snap", parent=> "nvme-snap",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 0, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["rdma-core", "spdk"], exception => 0 },

        'mlnx-dpdk' =>
            { name => "mlnx-dpdk", parent=> "mlnx-dpdk",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             do_not_release => 1,
             dist_req_inst => [],
             dist_req_build => [
                "$zlib_devel", "libpcap-devel", "python3-pyelftools",
                "meson", "ninja-build"
            ],
             dist_req_inst => [],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["rdma-core"], exception => 0 },

        'mlnx-dpdk-devel' =>
            { name => "mlnx-dpdk-devel", parent=> "mlnx-dpdk",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             do_not_release => 1,
             dist_req_inst => ["mlnx-dpdk"],
             dist_req_build => [
                "$zlib_devel", "libpcap-devel",
                "meson", "ninja-build"
             ],
             dist_req_inst => [],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["mlnx-dpdk"], exception => 0 },

        'mlnx-dpdk-doc' =>
            { name => "mlnx-dpdk-doc", parent=> "mlnx-dpdk",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 0, mode => "user",
             do_not_release => 1,
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["mlnx-dpdk"], exception => 0 },

        'mlnx-dpdk-tools' =>
            { name => "mlnx-dpdk-tools", parent=> "mlnx-dpdk",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 0, mode => "user",
             do_not_release => 1,
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["mlnx-dpdk"], exception => 0 },

        'mlnx-dpdk-debuginfo' =>
            { name => "mlnx-dpdk-debuginfo", parent=> "mlnx-dpdk",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 0, mode => "user",
             do_not_release => 1,
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["mlnx-dpdk"], exception => 0 },

        'openvswitch' =>
            { name => "openvswitch", parent=> "openvswitch",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => ["desktop-file-utils", "groff", "graphviz", "openssl-devel", "selinux-policy-devel", "python3-sphinx", "$python_twisted", "$python_zope_interface", "libcap-ng-devel", "$python_six", "unbound", "unbound-devel", "libpcap-devel", "$python3_devel", "$python_rpm_macros", "libtool"],
             dist_req_inst => ["openssl", "unbound"],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["rdma-core"], exception => 0 },

        'openvswitch-devel' =>
            { name => "openvswitch-devel", parent=> "openvswitch",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => ["openvswitch"], exception => 0 },

        'openvswitch-ipsec' =>
            { name => "openvswitch-ipsec", parent=> "openvswitch",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => ["openvswitch"], exception => 0 },

        'openvswitch-selinux-policy' =>
            { name => "openvswitch-selinux-policy", parent=> "openvswitch",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => ["openvswitch"], exception => 0 },

        'python-openvswitch' =>
            { name => "python-openvswitch", parent=> "openvswitch",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_build => [],
             dist_req_inst => ["python", "python-six"],
             ofa_req_build => [],
             ofa_req_inst => ["openvswitch"], exception => 0 },

        'python3-openvswitch' =>
            { name => "python3-openvswitch", parent=> "openvswitch",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_build => [],
             dist_req_inst => ["$python", "python3-six"],
             ofa_req_build => [],
             ofa_req_inst => ["openvswitch"],
             exception => 0
             },

        'openvswitch-test' =>
            { name => "openvswitch-test", parent=> "openvswitch",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => ["openvswitch"], exception => 0 },

        'network-scripts-openvswitch' =>
            { name => "network-scripts-openvswitch", parent=> "openvswitch",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_build => [],
             dist_req_inst => ["network-scripts"],
             ofa_req_build => [],
             ofa_req_inst => ["openvswitch"],
             exception => 0
             },

        'spdk' =>
            { name => "spdk", parent=> "spdk",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_build => ["CUnit-devel", "libaio-devel", "openssl-devel", "libuuid-devel", "libiscsi-devel", "python-pep8", "lcov", "clang-analyzer", "graphviz", "python-configshell", "ncurses-devel"],
             dist_req_inst => ["python", "sg3_utils", "avahi"],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["rdma-core"], exception => 0 },

        'mlnx-ethtool' =>
            { name => "mlnx-ethtool", parent=> "mlnx-ethtool",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [$libmnl],
             dist_req_build => ["$pkgconfig", "libmnl-devel"],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => [], exception => 0 },
        'mlnx-ethtool-debuginfo' =>
            { name => "mlnx-ethtool-debuginfo", parent=> "mlnx-ethtool",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => ["mlnx-ethtool"], exception => 0 },
        'mlnx-iproute2' =>
            { name => "mlnx-iproute2", parent=> "mlnx-iproute2",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 1, mode => "user",
             dist_req_inst => [$libmnl],
             dist_req_build => [$iptables_devel, $elfutils_devel, "libmnl-devel", $libdb_devel, "libselinux-devel", "bison", "flex"],
             ofa_req_build => [],
             ofa_req_inst => [], exception => 0 },
        'multiperf' =>
            { name => "multiperf", parent=> "multiperf",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 0, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["libibverbs"], exception => 0 },
        'multiperf-debuginfo' =>
            { name => "multiperf-debuginfo", parent=> "multiperf",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 0, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => ["rdma-core-devel"],
             ofa_req_inst => ["multiperf", "libibverbs"], exception => 0 },

        # this package is listed here only for uninstall and --without.. flag support
        'mlnx-fw-updater' =>
            { name => "mlnx-fw-updater", parent=> "mlnx-fw-updater",
             selected => 0, installed => 0, rpm_exist => 0,
             available => 0, mode => "user",
             dist_req_inst => [],
             dist_req_build => [],
             dist_req_inst => [],
             ofa_req_build => [],
             ofa_req_inst => [], exception => 0 },

        'rdma-core' =>
            { name => "rdma-core", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [
                "$cmake", "$libudev_devel", "$pkgconfig", "$python_rpm_macros",
                "$valgrind_devel", "$gcc", "$glibc_devel",
                "$libgcc", "$python_devel", "$python_cython",
                "$systemd_devel", "$perl_generators", "$perl_srpm_macros",
                "$libnl3_devel", "$annobin_gcc",
            ],
            dist_req_inst => [
                "$pkgconfig", "$libudev",
                    "$libnl3",
            ] ,
            ofa_req_build => [],
            ofa_req_inst => ["ofed-scripts"],
            exception => 0, configure_options => '' },
        'rdma-core-devel' =>
            { name => "rdma-core-devel", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core"],
            ofa_req_inst => ["rdma-core", "libibverbs", "librdmacm", "infiniband-diags", "libibumad"],
            exception => 0, configure_options => '' },
        'rdma-core-debuginfo' =>
            { name => "rdma-core-debuginfo", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core"],
            ofa_req_inst => ["rdma-core"],
            exception => 0, configure_options => '' },
        'python3-pyverbs' =>
            { name => "python3-pyverbs", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 0, mode => "user",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core"],
            ofa_req_inst => ["rdma-core", "libibverbs", "librdmacm", "infiniband-diags", "libibumad"],
            optional_subpackage => 1,
            exception => 0, configure_options => '' },

        'srp_daemon' =>
            { name => "srp_daemon", parent => "rdma-core",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => [],
            dist_req_inst => [],
            ofa_req_build => ["rdma-core", "rdma-core-devel"],
            ofa_req_inst => ["rdma-core", "libibumad", "libibverbs"],
            exception => 0, configure_options => '' },

        'ibarr' =>
            { name => "ibarr", parent => "ibarr",
            selected => 0, installed => 0, rpm_exist => 0,
            available => 1, mode => "user",
            dist_req_build => ["$libnl3_devel", "$pkgconfig", "$cmake"],
            dist_req_inst => ["$libnl3"],
            ofa_req_build => ["rdma-core-devel"],
            ofa_req_inst => ["libibverbs", "libibumad"],
            exception => 0,
            },
);

if ($DISTRO =~ /UOS20.1060/) {
    $packages_info{'sockperf'}{'available'} = 0;
    $packages_info{'knem'}{'available'} = 0;
}

if ($DISTRO =~ /MARINER|AZURE|AL23/) {
    $packages_info{"$mlnx_nvme_mods"}{'available'} = 0;
}

if ($DISTRO =~ /ANOLIS8.6|ALINUX/) {
    $packages_info{'knem'}{'available'} = 0;
}

foreach my $package (keys %packages_info) {
    if (not exists $packages_info{$package}{'configure_options'}) {
        $packages_info{$package}{'configure_options'} = '';
    }
}

foreach (qw/cma cuda gdrcopy ib ib knem rdmacm xpmem/) {
    $packages_info{"ucx-$_"}{"optional_subpackage"} = 1;
}

if ($DISTRO =~ /RHEL | FC | OL | BCLINUX/x and $DISTRO !~ /RHEL6 | OL6/x) {
    push @{$packages_info{'mpi-selector'}{'dist_req_inst'}}, "perl-Getopt-Long";
}

foreach (keys %packages_info) {
    my $entry = $packages_info{$_};
    next unless ($entry->{'mode'} eq 'kernel');
    next unless ($DISTRO =~ /RHEL8 | OL8 | EULEROS2.0SP[89] | BCLINUX8 | OPENEULER | CTYUNOS| FC29 | FC3./x);

    push @{$entry->{'dist_req_build'}}, 'gdb-headless';
}

if ($build_only) {
    for my $p (qw/mlnx-ofa_kernel mlnx-ofa_kernel-devel mlnx_en mlnx-en mlnx-en-utils/) {
        push @{$packages_info{$p}{'dist_req_build'}}, 'cpio';
    }
}

my @hidden_packages = ();

my %MPI_SUPPORTED_COMPILERS = (gcc => 0, pgi => 0, intel => 0, pathscale => 0);

my %gcc = ('gcc' => 0, 'gfortran' => 0, 'g77' => 0, 'g++' => 0);
my %pathscale = ('pathcc' => 0, 'pathCC' => 0, 'pathf90' => 0);
my %pgi = ('pgf77' => 0, 'pgf90' => 0, 'pgCC' => 0);
my %intel = ('icc' => 0, 'icpc' => 0, 'ifort' => 0);

my $TOPDIR = $builddir . '/' . $PACKAGE . "_topdir";
if (non_root_user()) {
    use File::Temp;
    $TOPDIR = File::Temp::tempdir(
        TEMPLATE => "${PACKAGE}_topdir_XXXXXX", DIR => $TMPDIR);
}

rmtree ("$TOPDIR");
mkpath([$TOPDIR . '/BUILD' ,$TOPDIR . '/RPMS',$TOPDIR . '/SOURCES',$TOPDIR . '/SPECS',$TOPDIR . '/SRPMS']);

my $components_filter = get_components_filter(0, [@components]);

if ($config_given and $install_option) {
    print_and_log_colored("\nError: '-c' option can't be used with '--all|--hpc|--vma|--xlio|--basic|--bluefield'", 1, "RED");
    exit 1;
}

if ($config_given and not -e $config) {
    print_and_log_colored("$config does not exist", 1, "RED");
    exit 1;
}

if (not $config_given and -e $config) {
    move($config, "$config.save");
}

if ($quiet) {
    $verbose = 0;
    $verbose2 = 0;
    $verbose3 = 0;
}

my %ifcfg = ();
if ($config_net_given and not -e $config_net) {
    print_and_log_colored("$config_net does not exist", 1, "RED");
    exit 1;
}

my $eth_dev;
if ($config_net_given) {
    open(NET, "$config_net") or die "Can't open $config_net: $!";
    while (<NET>) {
        my ($param, $value) = split('=');
        chomp $param;
        chomp $value;
        my $dev = $param;
        $dev =~ s/(.*)_(ib[0-9]+)/$2/;
        chomp $dev;

        if ($param =~ m/IPADDR/) {
            $ifcfg{$dev}{'IPADDR'} = $value;
        }
        elsif ($param =~ m/NETMASK/) {
            $ifcfg{$dev}{'NETMASK'} = $value;
        }
        elsif ($param =~ m/NETWORK/) {
            $ifcfg{$dev}{'NETWORK'} = $value;
        }
        elsif ($param =~ m/BROADCAST/) {
            $ifcfg{$dev}{'BROADCAST'} = $value;
        }
        elsif ($param =~ m/ONBOOT/) {
            $ifcfg{$dev}{'ONBOOT'} = $value;
        }
        elsif ($param =~ m/LAN_INTERFACE/) {
            $ifcfg{$dev}{'LAN_INTERFACE'} = $value;
        }
        else {
            print_and_log_colored("Unsupported parameter '$param' in $config_net\n", $verbose2, "RED");
        }
    }
    close(NET);
}

sub sig_handler
{
    exit $ERROR;
}

sub getch
{
        my $c;
        system("stty -echo raw");
        $c=getc(STDIN);
        system("stty echo -raw");
        # Exit on Ctrl+c or Esc
        if ($c eq "\cC" or $c eq "\e") {
            print "\n";
            exit $ERROR;
        }
        print "$c\n";
        return $c;
}

sub get_rpm_name_arch($)
{
    my $rpm = shift;
    my $ret = `rpm --queryformat "[%{NAME}] [%{ARCH}]" -qp '$rpm' 2>/dev/null | grep -v Freeing`;
    chomp $ret;
    return $ret;
}

sub get_rpm_ver($)
{
    my $rpm = shift;
    my $ret = `rpm --queryformat "[%{VERSION}]\n" -qp '$rpm' 2>/dev/null | uniq`;
    chomp $ret;
    return $ret;
}

sub get_rpm_rel($)
{
    my $rpm = shift;
    my $ret = `rpm --queryformat "[%{RELEASE}]\n" -qp '$rpm' 2>/dev/null | uniq`;
    chomp $ret;
    return $ret;
}

# Get the version (or first, if there are multiple ones)
# Returns an empty string on error
sub get_rpm_ver_one($) {
    my $package = shift;
    my $version = `rpm -q --queryformat "[%{VERSION}]\n" "$package" 2>/dev/null | head -1`;
    chomp $version;
    return $version;
}

# Get RPM name and version of the INSTALLED package
sub get_rpm_ver_inst
{
    my $ret;
    $ret = `rpm --queryformat '[%{VERSION}]\n' -q @_ 2>/dev/null | uniq`;
    chomp $ret;
    return $ret;
}

sub get_rpm_rel_inst
{
    my $ret = `rpm --queryformat "[%{RELEASE}]\n" -q @_ 2>/dev/null | uniq`;
    chomp $ret;
    return $ret;
}

sub get_rpm_info($)
{
    my $rpm = shift;
    my $ret = `rpm --queryformat "[%{NAME}] [%{VERSION}] [%{RELEASE}] [%{DESCRIPTION}]" -qp '$rpm' 2>/dev/null`;
    chomp $ret;
    return $ret;
}

# Check if the rpm build macros set LT_SYS_LIBRARY_PATH in the configure
# macros. If so, openmpi needs to explicitly override it.
# This command should be run after build dependencies (specifically:
# rpm-build) were isntalled
sub get_openmpi_lt_sys_env_str() {
    my $env_str = '';
    # Note that the pipe should hide any actual errors from rpmbuild.
    my $rc = system("rpmbuild --eval '\%configure' 2>/dev/null | grep -q LT_SYS_LIBRARY_PATH");
    if ($rc == 0) {
        $env_str = "LT_SYS_LIBRARY_PATH='$prefix/lib64' ";
    }
    return $env_str;
}

# Check whether compiler $1 exist
sub set_compilers
{
    if (`which gcc 2> /dev/null`) {
        $gcc{'gcc'} = 1;
    }
    if (`which g77 2> /dev/null`) {
        $gcc{'g77'} = 1;
    }
    if (`which g++ 2> /dev/null`) {
        $gcc{'g++'} = 1;
    }
    if (`which gfortran 2> /dev/null`) {
        $gcc{'gfortran'} = 1;
    }

    if (`which pathcc 2> /dev/null`) {
        $pathscale{'pathcc'} = 1;
    }
    if (`which pathCC 2> /dev/null`) {
        $pathscale{'pathCC'} = 1;
    }
    if (`which pathf90 2> /dev/null`) {
        $pathscale{'pathf90'} = 1;
    }

    if (`which pgcc 2> /dev/null`) {
        $pgi{'pgcc'} = 1;
    }
    if (`which pgCC 2> /dev/null`) {
        $pgi{'pgCC'} = 1;
    }
    if (`which pgf77 2> /dev/null`) {
        $pgi{'pgf77'} = 1;
    }
    if (`which pgf90 2> /dev/null`) {
        $pgi{'pgf90'} = 1;
    }

    if (`which icc 2> /dev/null`) {
        $intel{'icc'} = 1;
    }
    if (`which icpc 2> /dev/null`) {
        $intel{'icpc'} = 1;
    }
    if (`which ifort 2> /dev/null`) {
        $intel{'ifort'} = 1;
    }
}

sub set_cfg
{
    my $srpm_full_path = shift @_;

    my $info = get_rpm_info($srpm_full_path);
    my $name = (split(/ /,$info,4))[0];
    my $version = (split(/ /,$info,4))[1];

    ( $main_packages{$name}{$version}{'name'},
      $main_packages{$name}{$version}{'version'},
      $main_packages{$name}{$version}{'release'},
      $main_packages{$name}{$version}{'description'} ) = split(/ /,$info,4);
      $main_packages{$name}{$version}{'srpmpath'}   = $srpm_full_path;

    print_and_log("set_cfg: " .
             "name: $name, " .
             "original name: $main_packages{$name}{$version}{'name'}, " .
             "version: $main_packages{$name}{$version}{'version'}, " .
             "release: $main_packages{$name}{$version}{'release'}, " .
             "srpmpath: $main_packages{$name}{$version}{'srpmpath'}\n", $verbose3);
}

# set a given property to a given value for all packages with name matching the given name.
sub set_property_for_packages_like
{
    my $name = shift @_;
    my $property = shift @_;
    my $value = shift @_;

    for my $key (keys %packages_info) {
        if ($key =~ /$name/) {
            $packages_info{$key}{$property} = $value;
        }
    }
}

sub disable_package
{
    my $key = shift;

    if (exists $packages_info{$key}) {
        $packages_info{$key}{'disable_package'} = 1;
        $packages_info{$key}{'available'} = 0;
        for my $requester (@{$packages_deps{$key}{'required_by'}}) {
            next if (exists $packages_info{$requester}{'disable_package'});
            disable_package($requester);
        }
    }
    # modules
    if (exists $kernel_modules_info{$key}) {
        $kernel_modules_info{$key}{'available'} = 0;
        for my $requester (@{$modules_deps{$key}{'required_by'}}) {
            disable_package($requester);
        }
    }

    if (not (exists $packages_info{$key} or exists $kernel_modules_info{$key})) {
        print_and_log_colored("Unsupported package: $key", (not $quiet), "YELLOW");
    }
}

# used for blocking packages that are replaced with rdma-core and vice versa
sub block_package
{
    my $key = shift;

    if (exists $packages_info{$key}) {
        $packages_info{$key}{'available'} = 0;
        $packages_info{$key}{'disabled'} = 1;
    }
    # modules
    if (exists $kernel_modules_info{$key}) {
        $kernel_modules_info{$key}{'available'} = 0;
        $kernel_modules_info{$key}{'disabled'} = 1;
    }
}

sub add_enabled_pkgs_by_user
{
    ##############
    # handle with/enable flags
    for my $key ( keys %force_enable_packages ) {
        next if (($key =~ /mlx-steering-dump/) and
                ($DISTRO =~ /RHEL7|SLES12|XenServer|OL7|BCLINUX2[12]|KYLIN|OPENEULER20.03|CTYUNOS|UOS|ALIOS|AL2/));
        ### fix kernel package name
	# backward compatibility for old rpm name
        if ($key =~ /kernel-ib/) {
            $key =~ s/kernel-ib/mlnx-ofa_kernel/g;
        }
        # if kmp not supported
        if ($key eq "mlnx-ofa_kernel" and not $kmp) {
            $key =~ s/mlnx-ofa_kernel/mlnx-ofa_kernel-modules/;
        }
        if ($key =~ m/kernel-mft-mlnx/ and not $kmp) {
            $key =~ s/-mlnx//;
        }
        if ($key eq "mlnx-en" and not $kmp) {
            $key = "mlnx_en";
        }
        if ($key eq "knem" and not $kmp) {
            # knem (user space) will be added automatically since knem-modules needs it
            $key = "knem-modules";
        }
        # if kmp supported
        if ($key eq "mlnx-ofa_kernel-modules" and $kmp) {
            $key =~ s/mlnx-ofa_kernel-modules/mlnx-ofa_kernel/;
        }
        if ($key =~ m/kernel-mft/ and $key !~ m/mlnx/ and $kmp) {
            $key .= "-mlnx";
        }
        if ($key eq "mlnx_en" and $kmp) {
            $key = "mlnx-en";
        }
        if ($key eq "knem-modules" and $kmp) {
            $key = "knem";
        }

        if ((exists $packages_info{$key}) and (exists $packages_info{$key}{'parent'})) {
            next if ($packages_info{$key}{'disabled'});
            $packages_info{$key}{'available'} = 1;
            push (@selected_by_user, $key);
        }
        if (exists $kernel_modules_info{$key}) {
            next if ($kernel_modules_info{$key}{'disabled'});
            $kernel_modules_info{$key}{'available'} = 1;
            push (@selected_modules_by_user , $key);
        }

        if (not (exists $packages_info{$key} or exists $kernel_modules_info{$key})) {
            print_and_log_colored("Unsupported package: $key", (not $quiet), "YELLOW");
        }
    }
}

sub check_autofconf
{
    my $VAR = shift;

    my $value = `tac ${kernel_sources}/include/*/autoconf.h 2>/dev/null | grep -m1 ${VAR} 2>/dev/null | sed -ne 's/.*\\\([01]\\\)\$/\\1/gp' 2>/dev/null`;
    chomp $value;
    if ($value eq "") {
        $value = 0;
    }

    return $value;
}

# Set packages availability depending OS/Kernel/arch
sub set_availability
{
    set_compilers();

    if ($build_dkms_packages) {
        $packages_info{"mlnx-ofa_kernel-devel"}{"available"} = 0;
    }

    if ($user_space_only) {
        $packages_info{"mlnx-ofa_kernel"}{"mode"} = "user";
        $packages_info{'ucx-xpmem'}{'available'} = 0;
        $packages_info{'ucx-knem'}{'available'} = 0;
        $packages_info{'kernel-mft-dkms'}{'available'} = 0;
    }

    if ($is_bf) {
        # Avoid rshim installation on BlueField
        $packages_info{'rshim'}{'available'} = 0;
    }

    if ($arch =~ m/aarch64/ and $DISTRO =~ /ALTERNATE/) {
        if ($with_bluefield) {
            $packages_info{'sockperf'}{'available'} = 0;
        }
    } else {
        $packages_info{'libpka'}{'available'} = 0;
        $packages_info{'spdk'}{'available'} = 0;
        $packages_info{'mlxbf-bootctl'}{'available'} = 0;
        $packages_info{'mlxbf-bootimages'}{'available'} = 0;
        $packages_info{'bfscripts'}{'available'} = 0;
    }

    if ($arch =~ m/aarch64/ and $DISTRO =~ m/ALTERNATE/) {
        $packages_info{'nvme-snap'}{'available'} = 1;
    }

    if ($DISTRO =~ /FC3[0-9] | RHEL7\.2/x) {
        $packages_info{'openvswitch'}{'available'} = 0;
    }
    if ($DISTRO !~ /RHEL[7-9] | RHEL10 | FC3[0-9] | OPENEULER20.03SP1 | ANOLIS | ALINUX | CTYUNOS/x or
        ($DISTRO =~ /ALINUX/ and $arch =~ /aarch64/)) {
        $packages_info{'openvswitch'}{'available'} = 0;
        for my $package (@dpdk_packages) {
            $packages_info{$package}{'available'} = 0;
        }
    }

    if ($arch =~ /ppc/) {
        for my $package (qw/
            ucx ucx-devel ucx-static ucx-cma ucx-cuda ucx-gdrcopy ucx-ib ucx-ib-mlx5
            ucx-knem ucx-rdmacm ucx-xpmem
            mpi-selector openmpi clusterkit
            /)
        {
            $packages_info{$package}{'available'} = 0;
        }
    }

    if ($DISTRO =~ /RHEL7\.[2-5]/ and $arch eq 'aarch64') {
        # gcc fails to build mlnx-dpdk 20
        my $gcc_rel = get_rpm_rel_inst('gcc');
        if ($gcc_rel < 39) {
            for my $package (@dpdk_packages) {
                $packages_info{$package}{'available'} = 0;
            }
        }
    }

    # require numa stuff where supported inbox
    if ($DISTRO =~ /RHEL|OL|FC|POWERKVM|EULER|BCLINUX|ALIOS|ALINUX|CTYUNOS/) {
        $have_numa_libs = 1;
        push(@{$packages_info{'openmpi'}{'dist_req_build'}}, 'numactl-devel');
        push(@{$packages_info{'ucx'}{'dist_req_build'}}, 'numactl-devel');
        push(@{$packages_info{'mlnx-dpdk'}{'dist_req_build'}}, 'numactl-devel');
        push(@{$packages_info{'openvswitch'}{'dist_req_build'}}, 'numactl-devel');
        push(@{$packages_info{"libibverbs"}{'dist_req_build'}}, "numactl-devel");

        my $numactl = "numactl";
        if ($DISTRO =~ /FC19|FC[2-9]|OL[7-9]|RHEL[7-9]|POWERKVM|EULER|BCLINUX|ALIOS/) {
            $numactl = "numactl-libs";
        }
        if ($DISTRO =~ /MARINER/) {
            $numactl = "libnuma-devel";
        }
        push(@{$packages_info{'openmpi'}{'dist_req_inst'}}, "$numactl");
        push(@{$packages_info{'ucx'}{'dist_req_inst'}}, "$numactl");
        push(@{$packages_info{'mlnx-dpdk'}{'dist_req_inst'}}, "$numactl");
        push(@{$packages_info{"libibverbs"}{'dist_req_inst'}}, "${numactl}");
    }

    # mvapich and openmpi
    if ($gcc{'gcc'}) {
        $packages_info{'openmpi'}{'available'} = 1;
    }

    if ($arch !~ m/x86_64|ppc64le|aarch|arm/ or $DISTRO =~ /XenServer/) {
        for my $package (@mpi_packages) {
            $packages_info{$package}{'available'} = 0;
        }
        $packages_info{'ucx'}{'available'} = 0;
        $packages_info{'ucx-devel'}{'available'} = 0;
        $packages_info{'ucx-static'}{'available'} = 0;
        $packages_info{'ucx-cma'}{'available'} = 0;
        $packages_info{'ucx-ib'}{'available'} = 0;
        $packages_info{'ucx-ib-mlx5'}{'available'} = 0;
        $packages_info{'ucx-rdmacm'}{'available'} = 0;
        $packages_info{"$knem_rpm"}{'available'} = 0;
        $packages_info{"$knem_mods"}{'available'} = 0 if ($knem_mods ne "");
    }

    if ($DISTRO =~ m/WINDRIVER6/) {
        $kernel_modules_info{'mthca'}{'available'} = 0;
        $kernel_modules_info{'nfsrdma'}{'available'} = 0;
        $packages_info{'mlnx-nfsrdma'}{'available'} = 0;
        $packages_info{'mlnx-nvme'}{'available'} = 0;
        $packages_info{'mlnx-rdma-rxe'}{'available'} = 0;
        $kernel_modules_info{'rds'}{'available'} = 0;
        $kernel_modules_info{'sdp'}{'available'} = 0;
        $kernel_modules_info{'srpt'}{'available'} = 0;
        $kernel_modules_info{'amso1100'}{'available'} = 0;
        $packages_info{'knem'}{'available'} = 0;
        $packages_info{"$knem_mods"}{'available'} = 0 if ($knem_mods ne "");
        for my $package (@mpi_packages) {
            $packages_info{$package}{'available'} = 0;
        }
        $packages_info{'ucx'}{'available'} = 0;
        $packages_info{'ucx-devel'}{'available'} = 0;
        $packages_info{'ucx-static'}{'available'} = 0;
        $packages_info{'ucx-cma'}{'available'} = 0;
        $packages_info{'ucx-ib'}{'available'} = 0;
        $packages_info{'ucx-ib-mlx5'}{'available'} = 0;
        $packages_info{'ucx-rdmacm'}{'available'} = 0;
        $packages_info{"$knem_rpm"}{'available'} = 0;
        $packages_info{"$knem_mods"}{'available'} = 0 if ($knem_mods ne "");
    }

    if (is_installed("cuda-cudart-devel-$cuda_version")) {
        $cuda_top_dir = "/usr/local/cuda-$cuda_dir_name";
        # So we don't diverge in the support matrix from the UBS, we won't build ucx-cuda.
        # We'll take whatever binaries the UBS builds for the UCX package.
        # $packages_info{'ucx-cuda'}{'available'} = 1;
        if ($DISTRO =~ /RHEL9|RHEL10/ && $arch !~ /ppc/) {
            $packages_info{'clusterkit'}{'available'} = 1;
        }
    }
    if (is_installed("gdrcopy-devel")) {
        # So we don't diverge in the support matrix from the UBS, we won't build ucx-gdrcopy.
        # We'll take whatever binaries the UBS builds for the UCX package.
        # $packages_info{'ucx-gdrcopy'}{'available'} = 1;
    }

    if ( not ($with_vma or $with_xlio) or ($arch !~ m/x86_64|ppc64|aarch64/) or $DISTRO =~ /XenServer|WINDRIVER6/) {
        for my $package (qw/
            libvma libvma-utils libvma-devel
            sockperf
            libxlio libxlio-utils libxlio-devel
        /) {
            $packages_info{$package}{'available'} = 0;
        }
    }

    if ($DISTRO =~ /XenServer/) {
        $packages_info{'opensm'}{'available'} = 0;
        $packages_info{'opensm-libs'}{'available'} = 0;
        $packages_info{'opensm-devel'}{'available'} = 0;
        $packages_info{'opensm-debuginfo'}{'available'} = 0;
        $packages_info{'opensm-static'}{'available'} = 0;
    }

    if ($rpm_distro =~ /rhel7u2 | sles12sp3 | xenserver/x) {
        $packages_info{'ibarr'}{'available'} = 0;
    }
    if (not (
        # White list of supported libxlio platforms:
        ($DISTRO =~ /RHEL9|OL8|CTYUNOS2[35]/) and ($arch =~ /x86_64|aarch64/)
        )
    ) {
        for my $package (qw/libxlio libxlio-utils libxlio-devel/) {
            $packages_info{$package}{'available'} = 0;
        }
    }

    if (($DISTRO !~ /RHEL8.[13468]$|OL8|RHEL9.[34]/) or ($arch !~ /x86_64|aarch64/)) {
        for my $package (qw/libvma libvma-utils libvma-devel/) {
             $packages_info{$package}{'available'} = 0;
        }
    }

    if ($kernel_only) {
        for my $package (qw/libxpmem libxpmem-devel/) {
            $packages_info{$package}{'available'} = 0;
        }
    }

    # disable sockperf on SLES due to build issues
    if ($DISTRO =~ /POWERKVM/) {
       $packages_info{'sockperf'}{'available'} = 0;
    }

    # enable fabric-collector only if --with-fabric-collector was given and the OS is supported.
    if ( not ($with_fabric_collector and
             ($DISTRO =~ /RHEL6/ or
              $DISTRO =~ /RHEL7/ or
              $rpm_distro =~ /sles12sp/)) ) {
       $packages_info{'fabric-collector'}{'available'} = 0;
       $packages_info{'fabric-collector-debuginfo'}{'available'} = 0;
       print_and_log_colored("Unsupported package: fabric-collector", (not $quiet), "YELLOW") if ($with_fabric_collector);
    }

    if ($arch =~ /arm|aarch/i) {
        $packages_info{'dapl'}{'available'} = 0;
        $packages_info{'dapl-devel'}{'available'} = 0;
        $packages_info{'dapl-devel-static'}{'available'} = 0;
        $packages_info{'dapl-utils'}{'available'} = 0;
        $packages_info{'dapl-debuginfo'}{'available'} = 0;
    }

    if ($kernel =~ /fbk/ or $arch =~ /arm|aarch/) {
       $kernel_modules_info{'sdp'}{'available'} = 0;
    }

    if ($kernel =~ /fbk36/) {
       $kernel_modules_info{'rds'}{'available'} = 0;
    }
    # turn on isert if we are on follow OS and arch
    if (not ($rpm_distro =~ /
            rhel7u[2-9] | rhel[89] | rhel10 |
            ol7u4 |
            sles12sp[1-5] | sles15sp | sles16sp |
            fc2[789] | fc3[0-9] |
            alinux | anolis
        /x and $kernel =~ /
            fbk16 | ^4\.[4-9] | ^4\.1[0-9]\. | ^4\.20 | ^[5-9] |
            ^4\.1\.12-94 |                                  # OL7.4
            3.10.0-.*el7 | 3.12.28-4 | 3.12.49- | 3.12.48- |
            4\.18\.0-.*el8
        /x
    )) {
       $kernel_modules_info{'isert'}{'available'} = 0;
       $packages_info{'isert'}{'available'} = 0;
    }
    if ($packages_info{isert}{'available'}) {
        my $CONFIG_ISCSI_TARGET = check_autofconf('CONFIG_ISCSI_TARGET');
        if ($CONFIG_ISCSI_TARGET ne "1") {
            $kernel_modules_info{'isert'}{'available'} = 0;
            $packages_info{isert}{'available'} = 0;
        }
    }

    if ($DISTRO =~ /RHEL6.4|RHEL6.5|RHEL7\.[0-5]/ and $arch =~ m/x86_64/) {
       $packages_info{'multiperf'}{'available'} = 1;
       $packages_info{'multiperf-debuginfo'}{'available'} = 1;
    }

    if ((($arch ne 'x86_64') or ($kernel !~ /
              ^4\.12\.14-94\b   # SLES12 SP4
            | ^4\.12\.14-12[0-2]\b  # SLES12 SP5
            | ^4\.12\.14-195\b  # SLES15 SP1
            | ^5\.3\.18-22\b    # SLES15SP2
            | ^5\.3\.18-57\b    # SLES15SP3
            | ^5\.14\.21-\b     # SLES15SP4
            | ^4\.19\.90-2107\b     # B|OL7|BCLINUX21|KYLIN|OPENEULER20.03CLINUX21.10
            | ^5\.10\.0-\b
            | ^3\.10\.0-(\d+)\b # RHEL 7.x
            | ^4\.18\.0-(\d+)\b # RHEL 8.x
            | ^5\.14\.0-(\d+)\b # RHEL 9.x
            | ^5\.13\b
            | ^5\.15\b
            | ^5\.17\b          # Latest rebase's base
            | ^5\.18\b
            | ^5\.19\b
            | ^6\.[0-4]\b
            | ^6\.12\b# Latest mainline
            /x
        ) or (
            # RHEL7: >= 7.5:
            ($kernel =~ /^3\.10\.0-(\d+)\b/) and ($1 < 862)
        ) or
        $rpm_distro =~ /euler|ol8/)
        and
      ($arch ne 'aarch64' or $DISTRO !~/RHEL9.[2-7]|RHEL8.10|RHEL8/))
      {
        $kernel_modules_info{'nfsrdma'}{'available'} = 0;
        $packages_info{'mlnx-nfsrdma'}{'available'} = 0;
      }

    if (not ($DISTRO =~ /RHEL7.[7-9]|RHEL8.[13]$|RHEL9.[1-4]|OL8.[14678]/x and $arch eq 'x86_64') and
        not ($DISTRO =~ /RHEL8.[468] | RHEL9 | CTYUNOS2[35]/x and $arch =~ /x86_64|aarch64/x )) {
        $packages_info{'dpcp'}{'available'} = 0;
        $packages_info{'dpcp-debuginfo'}{'available'} = 0;
    }

    if (not $with_fwctl) {
        $packages_info{'fwctl'}{'available'} = 0;
    }

    if (not $with_virtiofs or
        $kernel !~ /
          ^6\.[6-9]\b
        | ^6\.1[0-7]\b
        /x) {
        $packages_info{'virtiofs'}{'available'} = 0;
        $packages_info{'virtiofs-dkms'}{'available'} = 0;
    }

    my $CONFIG_NET_UDP_TUNNEL = check_autofconf('CONFIG_NET_UDP_TUNNEL');
    if ($kernel !~ /^[5-9]|^4\.[8-9]|^4\.1[0-9]\./ or $CONFIG_NET_UDP_TUNNEL ne "1") {
        $packages_info{'mlnx-rdma-rxe'}{'available'} = 0;
    }

    # turn off srp and iser if we are not on follow OS and arch
    if (not ($rpm_distro =~ /
            rhel6u[8-9] | rhel6u10 | rhel7u[1-9] | rhel[89] | rhel10 |
            ol6u9 | ol7u4 |
            sles12sp[1-5] | sles15sp | sles16sp |
            fc2[789] | fc3[0-9] |
            alinux | anolis
        /x and $kernel =~ /
            ^3.1[6-9] | ^[4-9] |
            3.10.0.*el7.* | 3.*-default | 2.6.32.*el6.*
        /x)
    ) {
        $kernel_modules_info{'srp'}{'available'} = 0;
        $packages_info{'srp'}{'available'} = 0;
        $kernel_modules_info{'iser'}{'available'} = 0;
        $packages_info{'iser'}{'available'} = 0;
    }
    if ($packages_info{iser}{'available'}) {
        my $CONFIG_ISCSI_TCP = check_autofconf('CONFIG_ISCSI_TCP');
        if ($CONFIG_ISCSI_TCP ne "1") {
            $kernel_modules_info{'iser'}{'available'} = 0;
            $packages_info{iser}{'available'} = 0;
        }
    }

    if ($arch =~ /ppc64/) {
        $kernel_modules_info{'srp'}{'available'} = 0;
        $packages_info{'srp'}{'available'} = 0;
    }

    # disable iproute2 for unsupported OSs
    if ($rpm_distro =~ /fc23|powerkvm|xenserver|windriver/) {
        $packages_info{'mlnx-iproute2'}{'available'} = 0;
    }

    if ($arch =~ /ppc64/ and $rpm_distro =~ /rhel7u[0-1]/) {
        $packages_info{'mlnx-iproute2'}{'available'} = 0;
    }

    if ($arch =~ /ppc64/ and $rpm_distro =~ /rhel7u[0-1]/) {
        $packages_info{'mlnx-iproute2'}{'available'} = 0;
    }

    if (not ($kernel =~ /^([4-9]|3\.1[0-9])/)) {
        $kernel_modules_info{'mlx5_fpga_tools'}{'available'} = 0;
    }

    if ($DISTRO =~ /EULER|MARINER2|AZURE|UOS20\.1060a|UOS20\.1070|TENCENTOS4\.4|KYLIN11|BCLINUX22/) {
        block_package("openmpi");
    }

    if ($DISTRO =~ /EULER/ and $DISTRO !~ /OPENEULER22.03SP[13]/) {
        block_package("ucx");
    }

    # make sure user cannot force adding disabled package using --with flag
    block_package("infiniband-diags-compat");
    block_package("infiniband-diags-guest");

    if ($DISTRO =~ /RHEL[89] | OL8 | FC2[6-9] | FC3. | SLES15 | SLES16/x or $with_pyverbs) {
        $packages_info{'python3-pyverbs'}{'available'} = 1;
    }

    ##############
    # handle without/disable flags
    if (keys %disabled_packages) {
        # build deps list
        for my $pkg (keys %packages_info) {
            my @packages = (@{$packages_info{$pkg}{'ofa_req_inst'}},
                @{$packages_info{$pkg}{'ofa_req_build'}});
            if ($build_only) {
                @packages = (@{$packages_info{$pkg}{'ofa_req_build'}});
            }
            for my $req (@packages) {
                next if not $req;
                push (@{$packages_deps{$req}{'required_by'}}, $pkg);
            }
        }
        for my $mod (keys %kernel_modules_info) {
            for my $req ( @{ $kernel_modules_info{$mod}{'requires'} } ) {
                next if not $req;
                push (@{$modules_deps{$req}{'required_by'}}, $mod);
            }
        }
        # disable packages
        for my $key ( keys %disabled_packages ) {
            disable_package($key);
        }
    }
    # end of handle without/disable flags

    #
    # keep this at the end of the function.
    #
    # debuginfo RPM currently are not supported on SuSE and Ubuntu
    if ($DISTRO =~ m/SLES|SUSE|WINDRIVER6|POWERKVM/) {
        for my $package (@all_packages) {
            if ($package =~ m/-debuginfo/) {
                $packages_info{$package}{'available'} = 0;
            }
        }
    }

    my $glibc_ver_str = get_rpm_ver_one($glibc_devel);
    my @glibc_ver = split(/[.]/, $glibc_ver_str);
    if (($glibc_ver[0] < 2) or
        (($glibc_ver[0] == 2) and $glibc_ver[1] < 15)
    ) {
        $packages_info{'ucx-cma'}{'available'} = 0;
    }

    add_enabled_pkgs_by_user();
}

# Some packages may be rebuilt for a different kernel version.
# Therefore we only allow them if the kernel version encoded in the rpm
# release string matches the kernel version we target.
sub check_kernel_matches($$) {
    my ($rpm_arch, $kernel_from_release) = @_;

    if ($rpm_arch ne $target_cpu) {
        return 0;
    }
    if ($DISTRO =~ /CTYUNOS22\./) {
        return ("$kernel_from_release" eq "$kernel_rel.ctl2");
    }

    my $bclinux_extra_str = "oe1.bclinux";
    if (($DISTRO =~ /BCLINUX/) and ($kernel_rel !~/$bclinux_extra_str/)) {
        if ("$kernel_from_release" eq "$kernel_rel.$bclinux_extra_str") {
            return 1;
        }
    }
    if ("$kernel_from_release.ky10" eq "$kernel_rel") {
        return 1;
    }
    return ("$kernel_from_release" eq "$kernel_rel");
}

# Set rpm_exist parameter for existing RPMs
sub set_existing_rpms
{
    # Check if the ofed-scripts RPM exist and its prefix is the same as required one
    my $scr_rpm = '';
    my $arch = $target_cpu;

    $scr_rpm = <$RPMS/ofed-scripts-[0-9]*.$arch.rpm>;
    if ( -e $scr_rpm ) {
        my $current_prefix = `rpm -qlp '$scr_rpm' 2>/dev/null | grep ofed_info | sed -e "s@/bin/ofed_info@@"`;
        chomp $current_prefix;
        print_and_log("Found $scr_rpm. Its installation prefix: $current_prefix\n", $verbose2);
        if (not $current_prefix eq $prefix) {
            print_and_log("Required prefix is: $prefix\n", $verbose2);
            print_and_log("Going to rebuild RPMs from scratch\n", $verbose2);
            return;
        }
    }

    for my $binrpm (<$RPMS/*.rpm>) {
        my ($rpm_name, $rpm_arch) = (split ' ', get_rpm_name_arch($binrpm));
        my $ver = get_rpm_ver($binrpm);
        my $ver_no_rel = $ver; #version without kernel release
        $ver_no_rel =~ s/_.*//g;
        my $kernel_from_release = get_rpm_rel($binrpm);
        chomp $kernel_from_release;
        $kernel_from_release =~ s/.*\.kver\.//g;
        $kernel_from_release =~ s/_OFED.*//g;

        if ($DISTRO =~ /XenServer/ and $arch eq "i386" and $rpm_name =~ /ofa_kernel/) {
            $arch = 'i686';
        }
        if ($rpm_name =~ /knem-modules|kernel-mft|iser|srp$|mlnx_en|mlnx-nfsrdma|mlnx-nvme|virtiofs/ and
            $rpm_name !~ /kmp|kmod|dkms/) {
            if ($kmp) {
                print_and_log("KMP=1 -> ignoring $rpm_name $ver\n", $verbose2);
                next;
            }
            if (check_kernel_matches($rpm_arch, $kernel_from_release)) {
                $main_packages{$rpm_name}{$ver}{'rpmpath'} = $binrpm;
                $packages_info{$rpm_name}{$ver}{'rpm_exist'} = 1;
                print_and_log("$rpm_name $ver RPM exist\n", $verbose2);
            }

        } elsif ($rpm_name =~ /mlnx-ofa_kernel-modules/) {
            if (check_kernel_matches($rpm_arch, $kernel_from_release)) {
                $main_packages{$rpm_name}{$ver}{'rpmpath'} = $binrpm;
                $packages_info{$rpm_name}{$ver}{'rpm_exist'} = 1;
                print_and_log("$rpm_name $ver RPM exist\n", $verbose2);
            }

        # W/A for kmp packages that has only kmod and kmp rpms
        } elsif ($rpm_name =~ /kmp|kmod/ and
                 $rpm_name =~ /kernel-mft-mlnx|iser|srp|mlnx-en|mlnx-nfsrdma|mlnx-nvme|mlnx-rdma-rxe/) {
                if (not $kmp) {
                    print_and_log("KMP=0 -> ignoring $rpm_name $ver\n", $verbose2);
                    next;
                }
                my $pkname = $rpm_name;
                $pkname =~ s/kmod-//g;
                $pkname =~ s/-kmp.*//g;
                $main_packages{$rpm_name}{$ver}{'rpmpath'} = $binrpm;
                $packages_info{"$rpm_name"}{$ver}{'rpm_exist'} = 1;
                $packages_info{"$pkname"}{$ver}{'rpm_exist'} = 1;
                $packages_info{"$pkname"}{$ver_no_rel}{'rpm_exist'} = 1;
                print_and_log("$rpm_name $ver ($pkname) RPM exist\n", $verbose2);
        } else {
            if (($rpm_arch eq $arch or $rpm_arch eq "noarch") and
                 (($rpm_name !~ /iser|srp$|mlnx-nfsrdma|mlnx-nvme|virtiofs/) or
                  ($rpm_name =~ /-dkms/))) {
                $main_packages{$rpm_name}{$ver}{'rpmpath'} = $binrpm;
                $packages_info{$rpm_name}{$ver}{'rpm_exist'} = 1;
                if ($rpm_arch eq 'noarch') {
                    $packages_info{$rpm_name}{$ver}{'is_noarch'} = 1;
                }
                print_and_log("$rpm_name $ver RPM exist\n", $verbose2);
            }
        }
        $arch = $target_cpu;
    }
}

sub show_menu
{
    my $menu = shift @_;
    my $max_inp;

    print $clear_string;
    if ($menu eq "main") {
        print "$PACKAGE Distribution Software Installation Menu\n\n";
        print "   1) View $PACKAGE Installation Guide\n";
        print "   2) Install $PACKAGE Software\n";
        print "   3) Show Installed Software\n";
        print "   4) Configure IPoIB\n";
        print "   5) Uninstall $PACKAGE Software\n";
#        print "   6) Generate Supporting Information for Problem Report\n";
        print "\n   Q) Exit\n";
        $max_inp=5;
        print "\nSelect Option [1-$max_inp]:"
    }
    elsif ($menu eq "select") {
        print "$PACKAGE Distribution Software Installation Menu\n\n";
        print "   1) Basic ($PACKAGE modules and basic user level libraries)\n";
        print "   2) HPC ($PACKAGE modules and libraries, MPI and diagnostic tools)\n";
        print "   3) All packages (all of Basic, HPC)\n";
        print "   4) Customize\n";
        print "   5) Packages required by VMA (IB and Eth)\n";
        print "   6) Packages required by VMA IB\n";
        print "   7) Packages required by VMA Eth\n";
        print "   8) Packages required by Guest OS\n";
        print "   9) Packages required by Hypervisor OS\n";
        print "   10) Packages required by Bluefield OS\n";
        print "   11) Packages required by XLIO\n";
        print "\n   Q) Exit\n";
        $max_inp=11;
        print "\nSelect Option [1-$max_inp]:"
    }

    return $max_inp;
}

# Select package for installation
sub select_packages
{
    my $cnt = 0;
    if ($interactive) {
        open(CONFIG, ">$config") || die "Can't open $config: $!";;
        flock CONFIG, $LOCK_EXCLUSIVE;
        my $ok = 0;
        my $inp;
        my $max_inp;
        while (! $ok) {
            $max_inp = show_menu("select");
            $inp = getch();
            if ($inp =~ m/[qQ]/ || $inp =~ m/[Xx]/ ) {
                die "Exiting\n";
            }
            if (ord($inp) == $KEY_ENTER) {
                next;
            }
            if ($inp =~ m/[0123456789abcdefABCDEF]/)
            {
                $inp = hex($inp);
            }
            if ($inp < 1 || $inp > $max_inp)
            {
                print "Invalid choice...Try again\n";
                next;
            }
            $ok = 1;
        }
        if ($inp == $BASIC) {
            for my $package (@basic_user_packages, @basic_kernel_packages) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @basic_kernel_modules ) {
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }
        elsif ($inp == $HPC) {
            for my $package ( @hpc_user_packages, @hpc_kernel_packages ) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @hpc_kernel_modules ) {
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }
        elsif ($inp == $VMA) {
            for my $package ( @vma_user_packages, @vma_kernel_packages ) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @vma_kernel_modules ) {
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }
        elsif ($inp == $VMAVPI) {
            for my $package ( @vmavpi_user_packages, @vma_kernel_packages ) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @vma_kernel_modules ) {
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }
        elsif ($inp == $VMAETH) {
            for my $package ( @vmaeth_user_packages, @vma_kernel_packages ) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @vma_kernel_modules ) {
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }
        elsif ($inp == $XLIO) {
            for my $package ( @xlio_user_packages, @xlio_kernel_packages ) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @xlio_kernel_modules ) {
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }
        elsif ($inp == $GUESTOS) {
            for my $package ( @guest_kernel_packages,@guest_user_packages ) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
        }
        elsif ($inp == $HYPERVISOROS) {
            for my $package ( @hypervisor_kernel_packages,@hypervisor_user_packages) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available('srpmpath'));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
        }
        elsif ($inp == $BLUEFIELD) {
            for my $package ( @bluefield_kernel_packages,@bluefield_user_packages) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if ($kernel_only and $packages_info{$package}{'mode'} ne 'kernel');
                next if (not is_srpm_available('srpmpath'));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @bluefield_kernel_modules ) {
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }
        elsif ($inp == $ALL) {
            for my $package ( @all_packages, @hidden_packages ) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @kernel_modules ) {
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }
        elsif ($inp == $CUSTOM) {
            my $ans;
            for my $package ( @all_packages ) {
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if (not is_srpm_available($parent));
                print "Install $package? [y/N]:";
                $ans = getch();
                if ( $ans eq 'Y' or $ans eq 'y' ) {
                    print CONFIG "$package=y\n";
                    push (@selected_by_user, $package);
                    $cnt ++;

                    if ($package =~ /ofa_kernel/) {
                        # Select kernel modules to be installed
                        for my $module ( @kernel_modules, @tech_preview ) {
                            next if (not $kernel_modules_info{$module}{'available'});
                            print "Install $module module? [y/N]:";
                            $ans = getch();
                            if ( $ans eq 'Y' or $ans eq 'y' ) {
                                push (@selected_modules_by_user, $module);
                                print CONFIG "$module=y\n";
                            }
                        }
                    }
                }
                else {
                    print CONFIG "$package=n\n";
                }
            }
            print "Please enter the $PACKAGE installation directory: [$prefix]:";
            $ans = <STDIN>;
            chomp $ans;
            if ($ans) {
                $prefix = $ans;
                $prefix =~ s/\/$//;
            }
            print CONFIG "prefix=$prefix\n";
        }
        flock CONFIG, $UNLOCK;
    }
    else {
        if ($config_given) {
            open(CONFIG, "$config") || die "Can't open $config: $!";;
            while(<CONFIG>) {
                next if (m@^\s+$|^#.*@);
                my ($package,$selected) = (split '=', $_);
                chomp $package;
                chomp $selected;

                ### fix kernel package name
                # backward compatibility for old rpm name
                if ($package =~ /kernel-ib/) {
                    $package =~ s/kernel-ib/mlnx-ofa_kernel/g;
                }
                # if kmp not supported
                if ($package eq "mlnx-ofa_kernel" and not $kmp) {
                    $package =~ s/mlnx-ofa_kernel/mlnx-ofa_kernel-modules/;
                }
                if ($package =~ m/kernel-mft-mlnx/ and not $kmp) {
                    $package =~ s/-mlnx//;
                }
                if ($package eq "mlnx-en" and not $kmp) {
                    $package = "mlnx_en";
                }
                if ($package eq "knem" and not $kmp) {
                    # knem (user space) will be added automatically since knem-modules needs it
                    $package = "knem-modules";
                }
                # if kmp supported
                if ($package eq "mlnx-ofa_kernel-modules" and $kmp) {
                    $package =~ s/mlnx-ofa_kernel-modules/mlnx-ofa_kernel/;
                }
                if ($package =~ m/kernel-mft/ and $package !~ m/mlnx/ and $kmp) {
                    $package .= "-mlnx";
                }
                if ($package eq "mlnx_en" and $kmp) {
                    $package = "mlnx-en";
                }
                if ($package eq "knem-modules" and $kmp) {
                    $package = "knem";
                }

                print_and_log("$package=$selected\n", $verbose3);

                if ($package eq "prefix") {
                    $prefix = $selected;
                    $prefix =~ s/\/$//;
                    next;
                }

                if ($package eq "bonding_force_all_os") {
                    if ($selected =~ m/[Yy]|[Yy][Ee][Ss]/) {
                        $bonding_force_all_os = 1;
                    }
                    next;
                }

		if (substr($package,0,length("vendor_config")) eq "vendor_config") {
		       next;
		}

                if ($package eq "vendor_pre_install") {
		    if ( -e $selected ) {
			$vendor_pre_install = dirname($selected) . '/' . basename($selected);
		    }
		    else {
			print_and_log_colored("\nVendor script $selected is not found", (not $quiet), "RED");
			exit 1
		    }
                    next;
                }

                if ($package eq "vendor_post_install") {
		    if ( -e $selected ) {
			$vendor_post_install = dirname($selected) . '/' . basename($selected);
		    }
		    else {
			print_and_log_colored("\nVendor script $selected is not found", (not $quiet), "RED");
			exit 1
		    }
                    next;
                }

                if ($package eq "vendor_pre_uninstall") {
		    if ( -e $selected ) {
			$vendor_pre_uninstall = dirname($selected) . '/' . basename($selected);
		    }
		    else {
			print_and_log_colored("\nVendor script $selected is not found", (not $quiet), "RED");
			exit 1
		    }
                    next;
                }

                if ($package eq "vendor_post_uninstall") {
		    if ( -e $selected ) {
			$vendor_post_uninstall = dirname($selected) . '/' . basename($selected);
		    }
		    else {
			print_and_log_colored("\nVendor script $selected is not found", (not $quiet), "RED");
			exit 1
		    }
                    next;
                }

                if ($package eq "kernel_configure_options" or $package eq "OFA_KERNEL_PARAMS") {
                    $kernel_configure_options_global = $selected;
                    next;
                }

                if ($package eq "user_configure_options") {
                    $user_configure_options = $selected;
                    next;
                }

                if ($package =~ m/configure_options/) {
                    my $pack_name = (split '_', $_)[0];
                    $packages_info{$pack_name}{'configure_options'} = $selected;
                    next;
                }

                if (not $packages_info{$package}{'parent'} or $package =~ /iser|srp$/) {
                    my $modules = "@kernel_modules @tech_preview";
                    chomp $modules;
                    $modules =~ s/ /|/g;
                    if ($package =~ m/$modules/) {
                        if ( $selected eq 'y' ) {
                            if (not $kernel_modules_info{$package}{'available'}) {
                                print_and_log("$package is not available on this platform\n", (not $quiet));
                            }
                            else {
                                push (@selected_modules_by_user, $package);
                            }
                            next if ($package !~ /iser|srp/);
                        }
                    }
                    else {
                       print_and_log("Unsupported package: $package\n", (not $quiet));
                       next;
                    }
                }

                if (not $packages_info{$package}{'available'} and $selected eq 'y') {
                    print_and_log("$package is not available on this platform\n", (not $quiet));
                    next;
                }

                if ( $selected eq 'y' ) {
                    my $parent = $packages_info{$package}{'parent'};
                    if (not is_srpm_available($parent)) {
                        print_and_log("Unsupported package: $package\n", (not $quiet));
                        next;
                    }
                    push (@selected_by_user, $package);
                    print_and_log("select_package: selected $package\n", $verbose2);
                    $cnt ++;
                }
            }
        }
        else {
            open(CONFIG, ">$config") || die "Can't open $config: $!";
            flock CONFIG, $LOCK_EXCLUSIVE;
            if ($install_option eq 'all') {
                for my $package ( @all_packages ) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option eq 'guest-os') {
                for my $package ( @guest_kernel_packages,@guest_user_packages ) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @guest_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option eq 'hypervisor-os') {
                for my $package ( @hypervisor_kernel_packages,@hypervisor_user_packages ) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @hypervisor_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option eq 'bluefield') {
                for my $package ( @bluefield_user_packages, @bluefield_kernel_packages ) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if ($kernel_only and $packages_info{$package}{'mode'} ne 'kernel');
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @bluefield_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option eq 'hpc') {
                for my $package ( @hpc_user_packages, @hpc_kernel_packages ) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @hpc_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option =~ m/vma/) {
                my @list = ();
                if ($install_option eq 'vma') {
                    @list = (@vma_user_packages);
                } elsif ($install_option eq 'vmavpi') {
                    @list = (@vmavpi_user_packages);
                } elsif ($install_option eq 'vmaeth') {
                    @list = (@vmaeth_user_packages);
                }
                for my $package ( @list, @vma_kernel_packages ) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @vma_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option =~ m/xlio/) {
                for my $package (@xlio_user_packages, @xlio_kernel_packages) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @xlio_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option eq 'basic') {
                for my $package (@basic_user_packages, @basic_kernel_packages) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @basic_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option eq 'kernel-only') {
                for my $package ( @all_packages ) {
                    next if (not $packages_info{$package}{'available'});
                    next if (($packages_info{$package}{'mode'} ne 'kernel') && ($packages_info{$package}{'name'} ne 'kernel-mft-dkms'));
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option eq 'eth-only') {
                for my $package (@eth_user_packages, @eth_kernel_packages) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @basic_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            elsif ($install_option =~ m/dpdk/) {
                for my $package ( @dpdk_user_packages, @dpdk_kernel_packages ) {
                    next if (not $packages_info{$package}{'available'});
                    my $parent = $packages_info{$package}{'parent'};
                    next if (not is_srpm_available($parent));
                    push (@selected_by_user, $package);
                    print CONFIG "$package=y\n";
                    $cnt ++;
                }
                for my $module ( @dpdk_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    push (@selected_modules_by_user, $module);
                    print CONFIG "$module=y\n";
                }
            }
            else {
                print_and_log_colored("\nUnsupported installation option: $install_option", (not $quiet), "RED");
                exit 1;
            }
        }

        if ($with_bluefield and $install_option ne 'bluefield') {
            for my $package ( @bluefield_user_packages, @bluefield_kernel_packages ) {
                next if (grep /^$package$/, @selected_by_user);
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if ($kernel_only and $packages_info{$package}{'mode'} ne 'kernel');
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
            for my $module ( @bluefield_kernel_modules ) {
                next if (grep /^$module$/, @selected_modules_by_user);
                next if (not $kernel_modules_info{$module}{'available'});
                push (@selected_modules_by_user, $module);
                print CONFIG "$module=y\n";
            }
        }

        if ($with_ovs) {
            for my $package (@openvswitch_packages) {
                next if (grep /^$package$/, @selected_by_user);
                next if (not $packages_info{$package}{'available'});
                my $parent = $packages_info{$package}{'parent'};
                next if ($kernel_only and $packages_info{$package}{'mode'} ne 'kernel');
                next if (not is_srpm_available($parent));
                push (@selected_by_user, $package);
                print CONFIG "$package=y\n";
                $cnt ++;
            }
        }

        flock CONFIG, $UNLOCK;
    }
    close(CONFIG);


    return $cnt;
}

sub module_in_rpm
{
    my $name = shift @_;
    my $module = shift @_;
    my $ver = shift @_;
    my $ret = 1;
    my $package;

    my $version = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'version'};
    my $release = $kernel_rel;

    if ($name =~ /-modules/ or (not $kmp)) {
        $release = "$main_packages{$packages_info{$name}{'parent'}}{$ver}{'release'}" . ".kver.$kernel_rel" ;
    }

    my $arch = $target_cpu;
    if ($DISTRO =~ /XenServer/ and $target_cpu eq "i386" and $name =~ /ofa_kernel/) {
        $arch = 'i686';
    }

    if ($name =~ /mlnx-ofa_kernel/) {
        $package = $main_packages{$name}{$ver}{'rpmpath'};
    } else {
        $package = "$RPMS/$name-$version-$release.$arch.rpm";
        if (not -e "$package" and -e "$RPMS/$name-$version-$kernel_rel.$arch.rpm") {
            $package = "$RPMS/$name-$version-$kernel_rel.$arch.rpm";
        }
    }
    chomp $package;

    if (not -e $package) {
        print_and_log("is_module_in_rpm: $package not found\n", $verbose);
        return 1;
    }

    if ($module eq "nfsrdma") {
        $module = "rpcrdma";
    } elsif ($module eq "e_ipoib") {
        $module = "eth_ipoib";
    } elsif ($module eq "amso1100") {
        $module = "iw_c2";
    }

    open(LIST, "rpm -qlp $package 2>/dev/null |") or die "Can't run 'rpm -qlp $package': $!\n";
    while (<LIST>) {
        if (/$module[a-z_]*.ko/) {
            print_and_log("is_module_in_rpm: $module $_\n", $verbose3);
            $ret = 0;
            last;
        }
    }
    close LIST;

    if ($ret) {
        print_and_log("$module not in $package\n", $verbose2);
    }

    return $ret;
}

sub mark_for_uninstall
{
    my $package = shift @_;
    if (not $selected_for_uninstall{$package}) {
        push (@dependant_packages_to_uninstall, "$package");
        $selected_for_uninstall{$package} = 1;
        my $pname = $package;
        $pname =~ s@-[0-9].*@@g;
        if (not (exists $packages_info{$pname} or $pname =~ /mlnx-ofed- | doca-/x)) {
            $non_ofed_for_uninstall{$pname} = 1;
        }
    }
}

my %check_uninstall = ();
sub get_requires
{
    my $package = shift @_;

    # Strip RPM version
    my $pname = `rpm -q --queryformat "[%{NAME}]" $package 2>/dev/null`;
    chomp $pname;

    if ($check_uninstall{$package}) {
        return; # already checked here
    }
    $check_uninstall{$package} = 1;

    if ($pname eq "rdma") {
        # don't remove packages that needs rdma package
        return;
    }

    my @what_requires = `rpm -q --whatrequires $pname 2> /dev/null | grep -v "no package requires" 2> /dev/null`;

    for my $pack_req (@what_requires) {
        chomp $pack_req;
        print_and_log("get_requires: $pname is required by $pack_req\n", $verbose2);
        get_requires($pack_req);
        mark_for_uninstall($pack_req);
    }
}

# return 0 if pacakge not selected
# return 1 if pacakge selected
sub select_dependent
{
    my $package = shift @_;

    if ($user_space_only and ($packages_info{$package}{'mode'} eq 'kernel')) {
        print_and_log("select_dependent: in user-space-only mode, skipping kernel package: $package\n", $verbose2);
        return 0;
    }

    my $pname = $packages_info{$package}{'parent'};
    for my $ver (keys %{$main_packages{$pname}}) {
        # prevent loop
        if (not exists $packages_info{$package}{'entered_select_dependent'}) {
            $packages_info{$package}{'entered_select_dependent'}  = 1;
        } else {
            return 0 if (not $packages_info{$package}{'available'});
            my $parent = $packages_info{$package}{'parent'};
            return 0 if (not $main_packages{$parent}{$ver}{'srpmpath'});
            return 1;
        }

        if ( (not $packages_info{$package}{$ver}{'rpm_exist'})) {
            for my $req ( @{ $packages_info{$package}{'ofa_req_build'} } ) {
                next if not $req;
                # W/A for -p option and --user-space-only
                if ($req eq "mlnx-ofa_kernel-devel" and $print_available) {
                    next;
                }
                print_and_log("resolve_dependencies: $package requires $req for rpmbuild\n", $verbose2);
                my $req_selected = 0;
                if ($packages_info{$req}{'available'}) {
                    if (not $packages_info{$req}{'selected'}) {
                        $req_selected = select_dependent($req);
                    } else {
                        $req_selected = 1;
                    }
                }
                # Check if this is a strict requirment
                if (not $req_selected and not grep( /^$req$/, @{ $packages_info{$package}{'soft_req'} } )) {
                    print_and_log("select_dependent: $req requiement not satisfied for $package, skipping it\n", $verbose2);
                    $packages_info{$package}{'available'} = 0;
                    if (not $packages_info{$package}{"optional_subpackage"}) {
                        $packages_info{$pname}{'available'} = 0;
                    }
                    return 0;
                }
            }
        }

        for my $req ( @{ $packages_info{$package}{'ofa_req_inst'} } ) {
            next if not $req;
            next if $build_only;
            print_and_log("resolve_dependencies: $package requires $req for rpm install\n", $verbose2);
            my $req_selected = 0;
            if ($packages_info{$req}{'available'}) {
                if (not $packages_info{$req}{'selected'}) {
                    $req_selected = select_dependent($req);
                } else {
                    $req_selected = 1;
                }
            }
            if (not $req_selected and not grep( /^$req$/, @{ $packages_info{$package}{'soft_req'} } )) {
                print_and_log("select_dependent: $req requiement not satisfied for $package, skipping it\n", $verbose2);
                $packages_info{$package}{'available'} = 0;
                return 0;
            }
        }

        if (not $packages_info{$package}{'selected'}) {
            return 0 if (not $packages_info{$package}{'available'});
            my $parent = $packages_info{$package}{'parent'};
            return 0 if (not $main_packages{$parent}{$ver}{'srpmpath'});
            $packages_info{$package}{'selected'} = 1;
            push (@selected_packages, $package);
            print_and_log("select_dependent: Selected package $package\n", $verbose2);
            return 1;
        }
    }
    # if we get here, then nothing got selected.
    return 0;
}

sub select_dependent_module
{
    my $module = shift @_;

    if ($user_space_only and $print_available) {
        print_and_log("select_dependent_module in user-space-only and print_available mode, skipping kernel module: $module\n", $verbose2);
        return;
    }

    if (not $kernel_modules_info{$module}{'available'}) {
        print_and_log("select_dependent_module: $module is not available, skipping it\n", $verbose2);
        return;
    }

    # prevent loop
    if (not exists $kernel_modules_info{$module}{'entered_select_dependent_module'}) {
        $kernel_modules_info{$module}{'entered_select_dependent_module'}  = 1;
    } else {
        return;
    }

    for my $req ( @{ $kernel_modules_info{$module}{'requires'} } ) {
        print_and_log("select_dependent_module: $module requires $req for rpmbuild\n", $verbose2);
        if (not $kernel_modules_info{$req}{'selected'}) {
            select_dependent_module($req);
        }
    }
    if (not $kernel_modules_info{$module}{'selected'}) {
        $kernel_modules_info{$module}{'selected'} = 1;
        push (@selected_kernel_modules, $module);
        print_and_log("select_dependent_module: Selected module $module\n", $verbose2);
    }
}

sub resolve_dependencies
{
    for my $package ( @selected_by_user ) {
            # Get the list of dependencies
            select_dependent($package);
            if (exists $standalone_kernel_modules_info{$package}) {
                for my $mod (@{$standalone_kernel_modules_info{$package}}) {
                    if ($kernel_modules_info{$mod}{'available'}) {
                        push (@selected_modules_by_user, $mod);
                    }
                }
            }
        }

    for my $module ( @selected_modules_by_user ) {
        select_dependent_module($module);
    }

    my @kernel_rpms = qw(mlnx-ofa_kernel-modules kmod-mlnx-ofa_kernel mlnx-ofa_kernel-kmp-default);
    for my $kernel_rpm ( @kernel_rpms ) {
        my $pname = $packages_info{$kernel_rpm}{'parent'};
        if ( not $pname and $kernel_rpm =~ m/mlnx-ofa_kernel/ ) {
           $pname = "mlnx-ofa_kernel";
        }
        for my $ver (keys %{$main_packages{$pname}}) {
            if ($packages_info{$kernel_rpm}{$ver}{'rpm_exist'}) {
                for my $module (@selected_kernel_modules) {
                    if (module_in_rpm($kernel_rpm, $module, $ver)) {
                        $packages_info{$kernel_rpm}{$ver}{'rpm_exist'} = 0;
                        $packages_info{'mlnx-ofa_kernel'}{$ver}{'rpm_exist'} = 0;
                        last;
                    }
                }
                if ($with_memtrack) {
                    if (module_in_rpm($kernel_rpm, "memtrack", $ver)) {
                        $packages_info{$kernel_rpm}{$ver}{'rpm_exist'} = 0;
                        $packages_info{'mlnx-ofa_kernel'}{$ver}{'rpm_exist'} = 0;
                        last;
                    }
                }
            }
        }
    }
}

sub check_linux_dependencies
{
    my $err = 0;
    my $p1 = 0;
    my $gcc_32bit_printed = 0;
	my %missing_packages = ();
    if (! $check_linux_deps) {
        return 0;
    }
    my $dist_req_build = 'dist_req_build';
    for my $package ( @selected_packages ) {
        my $pname = $packages_info{$package}{'parent'};
        for my $ver (keys %{$main_packages{$pname}}) {
                # Check rpmbuild requirements
                if ($package =~ /ofa_kernel|kernel-mft|knem|mlnx-en/) {
                    if (not $packages_info{$package}{$ver}{'rpm_exist'}) {
                        # kernel sources required
                        if (not exists $missing_packages{"$kernel_devel"}) {
                            if ( not -d "$kernel_sources/scripts" ) {
                                $missing_packages{"$kernel_devel"} = 1;
                                print_and_log_colored("$kernel_sources/scripts is required to build $package RPM.", 1, "RED");
                                print_and_log_colored("Please install the corresponding kernel-devel RPM.", 1, "RED");
                                $err++;
                            }
                        }
                        # from kernel 4.14 we need elf devel package when CONFIG_UNWINDER_ORC=y
                        if (not is_installed("$kernel_elfutils_devel") and not exists $missing_packages{"$kernel_elfutils_devel"}) {
                            if ( check_autofconf('CONFIG_STACK_VALIDATION') eq "1" and check_autofconf('CONFIG_UNWINDER_ORC') eq "1" and $kernel =~ /^[5-9]|^4\.[1-9][4-9]\./) {
                                $missing_packages{"$kernel_elfutils_devel"} = 1;
                                print_and_log_colored("$kernel_elfutils_devel is required to build $package RPM.", $verbose2, "RED");
                                $err++;
                            }
                        }
                    }
                }

                if (not is_installed("rpm-build")) {
                    print_and_log_colored("rpm-build is required to build OFED", $verbose2, "RED");
                    $missing_packages{"rpm-build"} = 1;
                    $err++;
                }

                if ($DISTRO !~ m/SLES|SUSE|WINDRIVER6|BLUENIX1/) {
                        for my $req (@kernel_req) {
                            if (not is_installed("$req")) {
                                print_and_log_colored("kernel requirement $req rpm is required to build $package", $verbose2, "RED");
                                $missing_packages{"$req"} = 1;
                                $err++;
                            }
                        }
                }

                if (not $packages_info{$package}{$ver}{'rpm_exist'}) {
                    for my $req ( @{ $packages_info{$package}{$dist_req_build} } ) {
                        my ($req_name, $req_version) = (split ('__',$req));
                        next if not $req_name;
                        print_and_log("check_linux_dependencies: $req_name  is required to build $package $ver\n", $verbose3);
                        my $is_installed_flag = is_installed($req_name);
                        if (not $is_installed_flag) {
                            print_and_log_colored("$req_name rpm is required to build $package $ver", $verbose2, "RED");
                            if ("$req_name" eq "cmake" and $req_version ne "") {
                                print_and_log_colored("$req_name >= $req_version is required to build $package", 1, "RED");
                            }
                            $missing_packages{"$req_name"} = 1;
                            $err++;
                        }
                        if ($req_version) {
                            my $inst_version = get_rpm_ver_inst($req_name);
                            if ("$req_name" eq "cmake" and ($inst_version lt $req_version or $inst_version =~ /not installed/)) {
                                $inst_version = `cmake --version 2>/dev/null | grep "cmake version" | sed -e "s/.*cmake version\\s*//g"`;
                                chomp $inst_version;
                            }
                            print_and_log("check_linux_dependencies: $req_name installed version $inst_version, required at least $req_version\n", $verbose3);
                            if ($inst_version lt $req_version) {
                                print_and_log_colored("$req_name-$req_version rpm is required to build $package $ver", $verbose2, "RED");
                                if ("$req_name" eq "cmake") {
                                    print_and_log_colored("$req_name >= $req_version is required to build $package", 1, "RED");
                                    $missing_packages{"$req_name"} = 1;
                                } else {
                                    $missing_packages{"$req_name-$req_version"} = 1;
                                }
                                $err++;
                            }
                        }
                    }
                }

                if ((($DISTRO =~ /RHEL7\.6/) and ($arch eq 'x86_64'))  or
                    (($DISTRO =~ /RHEL7\.6ALTERNATE/) and ($arch eq 'ppc64le'))
                ) {
                    # The test for gdrcopy earlier on failed.
                    # We will try to install cuda packages explicitly.
                    if ($packages_info{'gdrcopy'}{'available'}) {
                        print_and_log("check_linux_dependencies: gdrcopy is required for ucx, but not installed.\n", $verbose2, "RED");
                        $err++;
                    }
                }
                # Check installation requirements
                my @dist_req_inst_packages = @{ $packages_info{$package}{'dist_req_inst'} };
                if ($build_only or ((not $with_dkms_install) and $package =~ /-dkms/)) {
                    @dist_req_inst_packages = ();
                }
                for my $req (@dist_req_inst_packages) {
                    my ($req_name, $req_version) = (split ('__',$req));
                    next if not $req_name;
                    my $is_installed_flag = is_installed($req_name);
                    if (not $is_installed_flag) {
                        print_and_log_colored("$req_name rpm is required to install $package $ver", $verbose2, "RED");
                        $missing_packages{"$req_name"} = 1;
                        $err++;
                    }
                    if ($req_version) {
                        my $inst_version = get_rpm_ver_inst($req_name);
                        print_and_log("check_linux_dependencies: $req_name installed version $inst_version, required $req_version\n", $verbose3);
                        if ($inst_version lt $req_version) {
                            print_and_log_colored("$req_name-$req_version rpm is required to install $package $ver", $verbose2, "RED");
                            $missing_packages{"$req_name-$req_version"} = 1;
                            $err++;
                        }
                    }
                }
        }
    }
    if ($err) {
        # display a summary of missing packages
        if (keys %missing_packages) {
            print_and_log_colored("\nError: One or more required packages for installing OFED-internal are missing.", 1, "RED");
            print_and_log_colored("Please install the missing packages using your Linux distribution Package Management tool.", 1, "RED");
            print_and_log("Run:\n$package_manager install " . join(' ', (keys %missing_packages)) . "\n", 1);
        }
        exit $PREREQUISIT;
    }

    if ($check_deps_only) {
        print_and_log("All required packages are installed, the system is ready for $PACKAGE installation.\n", 1);
        exit 0;
    }
}

# Print the list of selected packages
sub print_selected
{
    print_and_log_colored("\nBelow is the list of ${PACKAGE} packages that you have chosen
    \r(some may have been added by the installer due to package dependencies):\n", 1, "GREEN");
    for my $package ( @selected_packages ) {
        print_and_log("$package\n", 1);
    }
    print_and_log("\n", 1);
}

my $ALIOS_GCC_PATH = "/apsara/alicpp/built/gcc-9.2.1/gcc-9.2.1/bin";
sub kernel_build_munge_path() {
    return '' unless ($DISTRO =~ /ALIOS/);

    my $cur_path = $ENV{'PATH'};
    my $new_path = "$ALIOS_GCC_PATH:$cur_path";
    return "env PATH=$new_path";
}

sub alios_remove_gcc_from_path() {
    return unless ($DISTRO =~ /ALIOS/);
    return unless $ENV{'PATH'} =~ m/$ALIOS_GCC_PATH/;

    $ENV{'PATH'} =~ s/$ALIOS_GCC_PATH://;
}

# Check the value of %dist, but only if needed and only once
my $RPMBUILD_DIST;
sub get_rpmbuild_dist() {
    return $RPMBUILD_DIST if ($RPMBUILD_DIST);

    my $dist  = `rpm --eval '%{?dist}'`;
    chomp $dist;

    $dist =~ s/^\.//; # remove a leading dot, if any:

    $RPMBUILD_DIST = $dist;
    return $dist;
}

sub build_kernel_rpm
{
    my $name = shift @_;
    my $ver = shift @_;
    my $cmd;
    my $res = 0;
    my $sig = 0;
    my $TMPRPMS;
    my $rpm_distro = get_rpmbuild_dist(); # overrides global
    my $dist_part = $rpm_distro ? ".$rpm_distro" : "";
    my $kernel_configure_options = $kernel_configure_options_global;

    $cmd = "rpmbuild --rebuild $rpmbuild_flags --define '_topdir $TOPDIR' --define '_sourcedir %{_topdir}/SOURCES' --define '_specdir %{_topdir}/SPECS' --define '_srcrpmdir %{_topdir}/SRPMS' --define '_rpmdir %{_topdir}/RPMS' ";

    if ($DISTRO =~ /OL8.[7-9]|OL8.10/) {
        $cmd = "source /opt/rh/gcc-toolset-11/enable; $cmd";
    } elsif ($DISTRO =~ /OL9.6/) {
        $cmd = "source /opt/rh/gcc-toolset-14/enable; $cmd";
    }

    if (not $build_dkms_packages) {
        $cmd .= " --with building-kmods"
    }

    my $orig_rel = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'release'};
    my $new_rel = "${orig_rel}.kver.${kernel_rel}";

    if ($name =~ /ofa_kernel/) {
        $kernel_configure_options .= " $packages_info{$name}{'configure_options'}";

	my $CONFIG_XFRM_OFFLOAD = check_autofconf('CONFIG_XFRM_OFFLOAD');
        my $CONFIG_INET_ESP_OFFLOAD = check_autofconf('CONFIG_INET_ESP_OFFLOAD');
        my $CONFIG_INET6_ESP_OFFLOAD = check_autofconf('CONFIG_INET6_ESP_OFFLOAD');

        for my $module ( @selected_kernel_modules ) {
            if ($module eq "core") {
                $kernel_configure_options .= " --with-core-mod --with-user_mad-mod --with-user_access-mod --with-addr_trans-mod";
            }
            elsif ($module eq "ipath") {
                $kernel_configure_options .= " --with-ipath_inf-mod";
            }
            elsif ($module eq "qib") {
                $kernel_configure_options .= " --with-qib-mod";
            }
            elsif ($module eq "srpt") {
                $kernel_configure_options .= " --with-srp-target-mod";
            }
            elsif ($module eq "mlx5_fpga_tools") {
                # Innova/FPGA FLEX supported from kernel 3.10 and up
                $kernel_configure_options .= " --with-innova-flex";

                # Innova/FPGA IPSec supported only in MLNX_OFED, from kernel 4.13 and up
                # and it requires some kernel configs to be enabled
                if ($kernel =~ /^[5-9]|^4\.1[3-9]\./) {
                    print_and_log("\n-W- CONFIG_XFRM_OFFLOAD is not enabled in the kernel, Cannot build mlx5_core with Innova support\n", $verbose) if ($CONFIG_XFRM_OFFLOAD ne "1");
                    print_and_log("\n-W- None of CONFIG_INET6_ESP_OFFLOAD and CONFIG_INET_ESP_OFFLOAD enabled in the kernel, Cannot build mlx5_core with Innova support\n", $verbose) if ($CONFIG_INET_ESP_OFFLOAD ne "1" and $CONFIG_INET6_ESP_OFFLOAD ne "1");

                    if ($CONFIG_XFRM_OFFLOAD eq "1" and
                        ($CONFIG_INET_ESP_OFFLOAD eq "1" or $CONFIG_INET6_ESP_OFFLOAD eq "1")) {
                        $kernel_configure_options .= " --with-innova-ipsec";
                    }
                }
            }
            else {
                $kernel_configure_options .= " --with-$module-mod";
                if ($module eq "mlx5" and $with_mlx5_ipsec eq "1") {
                    #ConnectX IPsec option.
                    if ($CONFIG_XFRM_OFFLOAD eq "1" and
                        $CONFIG_INET_ESP_OFFLOAD eq "1" and $CONFIG_INET6_ESP_OFFLOAD eq "1") {
                        $kernel_configure_options .= " --with-mlx5-ipsec";
                        print_and_log("\n-W- --with-mlx5-ipsec is enabled\n", $verbose);
                    }
                }
            }
        }

        if ($with_memtrack) {
                $kernel_configure_options .= " --with-memtrack";
        }

        if ($with_gds) {
                $kernel_configure_options .= " --with-gds";
        }

        if ($with_sf_cfg_drv) {
                $kernel_configure_options .= " --with-sf-cfg-drv";
        }

        # Should be last change of $kernel_configure_options:
        if ($kernel_extra_args) {
            $kernel_configure_options .= " $kernel_extra_args";
        }

        if ($DISTRO =~ /XenServer/ and $target_cpu eq "i386") {
            $cmd .= " --define '_target_cpu i686'";
        }
        $cmd .= " --nodeps";
        $cmd .= " --define '_dist $dist_part'" if $dist_part;
        $cmd .= " --undefine 'dist'" if $DISTRO =~ /UOS20.1060a|CTYUNOS22/;
        $cmd .= " --define 'configure_options $kernel_configure_options'";
        $cmd .= " --define 'KVERSION $kernel'";
        $cmd .= " --define 'K_SRC $kernel_sources'";
        $cmd .= " --define 'KMP 1'" if $kmp;
        $cmd .= " --define '_kmp_build_num .$kmp_build_num'" if ($kmp and $kmp_build_num ne "");
    }
    elsif ($name =~ /kernel-mft/) {
        $cmd .= " --define 'KVERSION $kernel'";
        $cmd .= " --define 'K_SRC $kernel_sources'";

        if ($DISTRO =~ /AL23/) {
            $cmd .= " --define '__requires_exclude ^kernel.*'";
        }

        # for backward compatablilty check if the src.rpm does not support setting _release
        my $_release_supported = 1;
        mkpath([$TOPDIR . 'mft_check/BUILD' ,$TOPDIR . 'mft_check/RPMS',$TOPDIR . 'mft_check/SOURCES',$TOPDIR . 'mft_check/SPECS',$TOPDIR . 'mft_check/SRPMS']);
        system("rpm -ihv --nodeps --define '_topdir $TOPDIR/mft_check' --define '_sourcedir %{_topdir}/SOURCES' --define '_specdir %{_topdir}/SPECS' --define '_srcrpmdir %{_topdir}/SRPMS' --define '_rpmdir %{_topdir}/RPMS' '$main_packages{$name}{$ver}{'srpmpath'}' >/dev/null 2>&1");
        if (`grep -w _release $TOPDIR/mft_check/SPECS/*spec 2>/dev/null` eq "") {
            print_and_log("kernel-mft does not support setting _release\n", $verbose3);
            $_release_supported = 0;
        }
        system("rm -rf $TOPDIR/mft_check >/dev/null 2>&1");

        $cmd .= " --define 'source 1'" if ($_release_supported or $kmp);
        $cmd .= " --define '_release $new_rel'" if not $kmp;
        $cmd .= " --define 'debug_package %{nil}'";
        $cmd .= " --define '_dist $dist_part'" if $kmp and $dist_part;
        $cmd .= " --undefine 'dist'" if $DISTRO =~ /UOS20.1060a|CTYUNOS22/;
        $cmd .= " --define 'KMP 1'" if $kmp;
        $cmd .= " --define '_kmp_build_num .$kmp_build_num'" if ($kmp and $kmp_build_num ne "");
    }
    elsif ($name =~ /knem | xpmem/x) {
        $cmd .= " --define 'KVERSION $kernel'";
        $cmd .= " --define 'K_SRC $kernel_sources'";
        $cmd .= " --define '_dist $dist_part'" if $dist_part;
        $cmd .= " --undefine 'dist'" if $DISTRO =~ /UOS20.1060a|CTYUNOS22/;
        $cmd .= " --define 'KMP 1'" if $kmp;
        $cmd .= " --define '_kmp_build_num .$kmp_build_num'" if ($kmp and $kmp_build_num ne "");
        if ($kernel_only and $name =~ /xpmem/) {
            $cmd .= " --with kernel_only";
        }
    }
    # Standalone module packages
    elsif ($name =~ /iser|srp|mlnx-nfsrdma|mlnx-nvme|mlnx-rdma-rxe|fwctl|virtiofs/) {
        $cmd .= " --define 'src_release $new_rel'" if not ($kmp or $build_dkms_packages);
        $cmd .= " --define 'KVERSION $kernel'";
        $cmd .= " --define 'K_SRC $kernel_sources'";
        $cmd .= " --define '_dist $dist_part'" if ($kmp or $build_dkms_packages) and $dist_part;
        $cmd .= " --undefine 'dist'" if $DISTRO =~ /UOS20.1060a|CTYUNOS22/;
        $cmd .= " --define 'KMP 1'" if ($kmp or $build_dkms_packages);
        $cmd .= " --define '_kmp_build_num .$kmp_build_num'" if ($kmp and $kmp_build_num ne "");
        if ($DISTRO =~ /ALINUX3\.12/) {
            $cmd .= " --define '__find_requires %{nil}'";
        }
        if ($name =~ /mlnx-nvme/) {
            if (($DISTRO =~ /RHEL7.[2-3]|RHEL6.3/ and $kernel =~ /3.10.0/) or
                ($DISTRO =~ /RHEL7.[4-5]/ and $kernel =~ /
                    3.10.0-123 | 3.10.0-229 | 3.10.0-327| 3.10.0-514
                /x
            )) {
                $cmd .= " --define 'NVME_HOST_WITHOUT_FC 1'";
            }
        }
    }
    elsif ($name =~ /mlnx-en|mlnx_en/) {
        $cmd .= " --define '_dist $dist_part'" if $dist_part;
        $cmd .= " --undefine 'dist'" if $DISTRO =~ /UOS20.1060a|CTYUNOS22/;
        $cmd .= " --define 'KVERSION $kernel'";
        $cmd .= " --define 'KSRC $kernel_sources'";
        $cmd .= " --define 'MEMTRACK $with_memtrack'";
        my $mlx4f = (grep( /^mlx4$/, @selected_kernel_modules )) ? "1" : "0";
        $cmd .= " --define 'MLX4 $mlx4f'";
        my $mlx5f = (grep( /^mlx5$/, @selected_kernel_modules )) ? "1" : "0";
        $cmd .= " --define 'MLX5 $mlx5f'";
        my $mlxfwf = (grep( /^mlxfw$/, @selected_kernel_modules )) ? "1" : "0";
        $cmd .= " --define 'MLXFW $mlxfwf'";
        $cmd .= " --define 'KMP 1'" if $kmp;
        $cmd .= " --define '_kmp_build_num .$kmp_build_num'" if ($kmp and $kmp_build_num ne "");
    }

    if ($name =~ /xpmem/ and $DISTRO =~ /RHEL7/ and $arch ne 'x86_64') {
        # Remove -Wl,-z,relro from __global_ldflags. Seems to break ld
        # there:
        $cmd .= " --define '__global_ldflags %{_hardened_ldflags}'";
    }
    $cmd .= " --define '_prefix $prefix'";

    $cmd .= " '$main_packages{$name}{$ver}{'srpmpath'}'";

    my $munge_path_str = kernel_build_munge_path();
    $cmd = "$munge_path_str $cmd";

    if (exists $package_pre_build_script{$name}) {
        print_and_log("Running $name pre build script: $package_pre_build_script{$name}\n", $verbose);
        ex("$package_pre_build_script{$name}");
    }

    print_and_log("Running $cmd\n", $verbose);
    system("echo $cmd > $ofedlogs/$name-$ver.rpmbuild.log 2>&1");
    system("$cmd >> $ofedlogs/$name-$ver.rpmbuild.log 2>&1");
    $res = $? >> 8;
    $sig = $? & 127;
    if ($sig or $res) {
        print_and_log_colored("Failed to build $name $ver RPM", 1, "RED");
        addSetupInfo ("$ofedlogs/$name-$ver.rpmbuild.log");
        print_and_log_colored("See $ofedlogs/$name-$ver.rpmbuild.log", 1, "RED");
        exit 1;
    }

    my $arch = $target_cpu;
    if ($DISTRO =~ /XenServer/ and $target_cpu eq "i386" and $name =~ /ofa_kernel/) {
        $arch = 'i686';
    }
    $TMPRPMS = "$TOPDIR/RPMS/$arch";
    chomp $TMPRPMS;

    print_and_log("TMPRPMS $TMPRPMS\n", $verbose2);

    for my $myrpm ( <$TMPRPMS/*.rpm> ) {
        print_and_log("Created $myrpm\n", $verbose2);
        if ((not $build_dkms_packages) && $myrpm =~ /-dkms/) {
            print_and_log("Remove dkms rpm in a non-dkms build: $myrpm\n", $verbose);
            unlink $myrpm;
            next;
        }
        if ($name =~ /ofa_kernel/) {
            system("rpm -qlp '$myrpm' 2>/dev/null | grep lib.modules | awk -F '/' '{print\$4}' | sort -u >> $RPMS/.supported_kernels");
        }
        if ($name =~ /knem/) {
            $knem_prefix = `rpm -qlp '$myrpm' 2>/dev/null | grep -w "sbin\$" 2>/dev/null | sed -e "s@/sbin@@"`;
            chomp $knem_prefix;
        }
        my ($myrpm_name, $myrpm_arch) = (split ' ', get_rpm_name_arch($myrpm));
        # W/A for kmp packages that has only kmod and kmp rpms
        if($kmp and $myrpm_name =~ /kmp|kmod/ and
           $myrpm_name =~ /kernel-mft-mlnx|iser|srp|mlnx-en|mlnx-nfsrdma|mlnx-nvme|mlnx-rdma-rxe|fwctl|virtiofs/) {
            $myrpm_name =~ s/kmod-//g;
            $myrpm_name =~ s/-kmp.*//g;
        }
        my $ofa_dir = "$builddir$prefix/src/ofa_kernel/$arch/$kernel";
        delete $ENV{"OFA_DIR"};
        if (-d $ofa_dir) {
            $ENV{"OFA_DIR"} = $ofa_dir;
        }
        if ($build_only and $myrpm_name =~ /kernel.*devel/) {
            print_and_log("Extracting devel package $myrpm_name under builddir ($builddir) to allow other modules to build\n", $verbose2);
            system("rpm2cpio '$myrpm' | { cd '$builddir'; cpio -idu --quiet;}");
        }
        move($myrpm, "$RPMS");
        $packages_info{$myrpm_name}{$ver}{'rpm_exist'} = 1;
    }

    if (exists $package_post_build_script{$name}) {
        print_and_log("Running $name post build script: $package_post_build_script{$name}\n", $verbose);
        ex("$package_post_build_script{$name}");
    }
}

# Build RPM from source RPM
sub build_rpm
{
    my $name = shift @_;
    my $ver = shift @_;
    my $cmd;
    my $res = 0;
    my $sig = 0;
    my $TMPRPMS;

    my $ldflags;
    my $cflags;
    my $cppflags;
    my $cxxflags;
    my $fflags;
    my $ldlibs;
    my $openmpi_comp_env;
    my $parent = $packages_info{$name}{'parent'};
    my $srpmdir;
    my $srpmpath_for_distro;

    delete $ENV{"mellanox_autodetect"};

    print_and_log("Build $name $ver RPM\n", $verbose);

    my $pref_env = '';
    if ($prefix ne $default_prefix) {
        if ($parent !~ /openmpi/) {
            $ldflags .= "$optflags -L$prefix/lib64 -L$prefix/lib";
            $cflags .= "$optflags -I$prefix/include";
            $cppflags .= "$optflags -I$prefix/include";
        }
    }

    if (not $packages_info{$name}{$ver}{'rpm_exist'}) {

        if ($parent eq "ibacm") {
            if ($DISTRO eq "FC14") {
                $ldflags    = " -g -O2 -lpthread";
            }
            $pref_env .= "rdmascript=openibd"
        }

        if ($arch eq "ppc64") {
            if ($DISTRO =~ m/SLES/ and $dist_rpm_rel gt 15.2) {
                # SLES 10 SP1
                if ($parent =~ /openmpi/) {
                    $openmpi_comp_env .= ' LDFLAGS="-m64 -O2 -L/usr/lib/gcc/powerpc64-suse-linux/4.1.2/64"';
                }
            }
            else {
                if ($parent !~ /ibutils|ibdump/) {
                    $ldflags    .= " $optflags -m64 -g -O2 -L/usr/lib64";
                    $cflags     .= " $optflags -m64 -g -O2";
                    $cppflags   .= " $optflags -m64 -g -O2";
                    $cxxflags   .= " $optflags -m64 -g -O2";
                    $fflags     .= " $optflags -m64 -g -O2";
                    $ldlibs     .= " $optflags -m64 -g -O2 -L/usr/lib64";
                }
            }
        }

        if ($DISTRO =~ m/MARINER/) {
            if ($parent =~ /libvma/) {
                $cflags .= " -Werror=discarded-qualifiers";
            }
        }

        if ( ($parent =~ /perftest|openmpi/) and ($DISTRO =~ /FC2[3-9] | FC3./x) ) {
            $ldflags    .= " -fPIC";
            $cflags     .= " -fPIC";
        }
        if ( ($parent =~ /openmpi/) and ($DISTRO =~ /BCLINUX8|BCLINUX2[12]|KYLIN|RHEL9|OL9|UOS|ANOLIS|CTYUNOS|OPENEULER20.03SP1|TENCENT|ALINUX|AL23/x) ) {
            $ldflags    .= " -fPIC";
            $pref_env   .= get_openmpi_lt_sys_env_str();
        }
        if ( ($parent =~ /mstflint/) and
            ($DISTRO =~ /RHEL8 | OL8 | EULEROS2.0SP[89] | BCLINUX8 | OPENEULER | FC2[3-9] | FC3./x) ) {
                # mstflint and openmpi unset optflags in the rpm build spec.
                # This has the odd side effect of disabling the hardening specs
                # at the build stage, but not on the link stage. This fails the basic
                # autoconf test of building a program.
                foreach (qw/redhat openEuler/) {
                    my $specs = "/usr/lib/rpm/$_/$_-hardened-cc1";
                    if (-e $specs) {
                            $cflags    .= " -specs=$specs";
                    }
                }
        }

        if ($ldflags) {
            $pref_env   .= " LDFLAGS='$ldflags'";
        }
        if ($cflags) {
            $pref_env   .= " CFLAGS='$cflags'";
        }
        if ($cppflags) {
            $pref_env   .= " CPPFLAGS='$cppflags'";
        }
        if ($cxxflags) {
            $pref_env   .= " CXXFLAGS='$cxxflags'";
        }
        if ($fflags) {
            $pref_env   .= " FFLAGS='$fflags'";
        }
        if ($ldlibs) {
            $pref_env   .= " LDLIBS='$ldlibs'";
        }

        if ($parent =~ /openmpi/) {
            $pref_env   .= " mellanox_autodetect='yes'";
        }

        $cmd = "$pref_env rpmbuild --rebuild $rpmbuild_flags --define '_topdir $TOPDIR' --define '_sourcedir %{_topdir}/SOURCES' --define '_specdir %{_topdir}/SPECS' --define '_srcrpmdir %{_topdir}/SRPMS' --define '_rpmdir %{_topdir}/RPMS' ";
        $cmd .= " --define 'dist %{nil}'";
        # bug in RH 8.0 rpmbuild will not work with --target parameter
        if ( ($parent =~ /mstflint|openmpi/) and ($DISTRO =~ /RHEL8 | OL8/x) ) {
            $cmd .= "";
        } else {
            $cmd .= " --target $target_cpu";
        }

        if ($name =~ /kernel-mft/) {
            if (not $build_dkms_packages) {
                $cmd .= " --with building-kmods"
            } else {
                $cmd .= " --with building-dkms";
            }
        }

        # Prefix should be defined per package
        if ($parent eq "ibutils2") {
            my $global_cflags = `rpm --eval '%{__global_cflags}'`;
            chomp $global_cflags;
            $cmd .= " --define '__global_cflags $global_cflags'";
            $cmd =~ s/-Wp,-D_FORTIFY_SOURCE=2//g;
            $cmd .= " --define '_prefix $ibutils2_prefix'";
            $cmd .= " --define '_with_umad $prefix'";
            $cmd .= " --define '__arch_install_post %{nil}'";
            $cmd .= " --define '_exec_prefix $ibutils2_prefix'";
            $cmd .= " --define '_sysconfdir $sysconfdir'";
            $cmd .= " --define '_usr $ibutils2_prefix'";
            $cmd .= " --define '_mandir $ibutils2_prefix/share/man'";
        }
        elsif ($parent eq "cc_mgr") {
            $packages_info{'cc_mgr'}{'configure_options'} .= " --with-ibutils2=$ibutils2_prefix";
            $packages_info{'cc_mgr'}{'configure_options'} .= " --with-osm=$prefix";
            $cmd .= " --define '_prefix $prefix'";
            $cmd .= " --define '_exec_prefix $prefix'";
            $cmd .= " --define '_sysconfdir $sysconfdir'";
            $cmd .= " --define '_usr $prefix'";
            $cmd .= " --define '__arch_install_post %{nil}'";
        }
        elsif ($parent eq "clusterkit") {
            $cmd .= " --with mpi";
            # build dep, must be available:
            my $mpiver = (keys %{$main_packages{'openmpi'}})[0];
            my $mpi_home = "$prefix/mpi/gcc/openmpi-$mpiver";
            my $conf_opts = "--with-mpi=$mpi_home";
            if ($cuda_top_dir) {
                $cmd .= " --with cuda";
                $conf_opts .= " --with-cuda=$cuda_top_dir";
            }
            $cmd .= " --define 'configure_options $conf_opts'";
            $cmd = "QA_RPATHS=2 LD_LIBRARY_PATH=$mpi_home/lib64:\$LD_LIBRARY_PATH " . $cmd;
        }
        elsif ($parent eq "ar_mgr") {
            $packages_info{'ar_mgr'}{'configure_options'} .= " --with-ibutils2=$ibutils2_prefix";
            $packages_info{'ar_mgr'}{'configure_options'} .= " --with-osm=$prefix";
            $cmd .= " --define '_prefix $prefix'";
            $cmd .= " --define '_exec_prefix $prefix'";
            $cmd .= " --define '_sysconfdir $sysconfdir'";
            $cmd .= " --define '_usr $prefix'";
            $cmd .= " --define '__arch_install_post %{nil}'";
        }
        elsif ($parent eq "dump_pr") {
            $packages_info{'dump_pr'}{'configure_options'} .= " --with-osm=$prefix";
            $cmd .= " --define '_prefix $prefix'";
            $cmd .= " --define '_exec_prefix $prefix'";
            $cmd .= " --define '_sysconfdir $sysconfdir'";
            $cmd .= " --define '_usr $prefix'";
            $cmd .= " --define '__arch_install_post %{nil}'";
        }
        elsif ($parent =~ /libxlio/x) {
            if ($packages_info{"dpcp"}{'selected'}) {
                $cmd .= " --define 'configure_options --with-dpcp'";
            }
        }
        elsif ($parent =~ /openmpi/) {
            my $use_default_rpm_opt_flags = 1;
            my $openmpi_ldflags = '';
            my $openmpi_wrapper_cxx_flags;
            my $openmpi_lib;

            if ($arch =~ m/x86_64|ppc64/) {
                $openmpi_lib = 'lib64';
            }
            else {
                $openmpi_lib = 'lib';
            }

            if ($compiler eq "gcc") {
                if ($gcc{'g++'}) {
                }
                else {
                    $openmpi_comp_env .= " --disable-mpi-cxx";
                }
                if ($gcc{'gfortran'}) {
                }
                elsif ($gcc{'g77'}) {
                    $openmpi_comp_env .= " F77=g77 --disable-mpi-f90";
                }
                else {
                    $openmpi_comp_env .= " --disable-mpi-f77 --disable-mpi-f90";
                }
            }

            if ($arch eq "ppc64") {
                # In the ppc64 case, add -m64 to all the relevant
                # flags because it's not the default.  Also
                # unconditionally add $OMPI_RPATH because even if
                # it's blank, it's ok because there are other
                # options added into the ldflags so the overall
                # string won't be blank.
                $openmpi_comp_env .= ' CFLAGS="-m64 -O2" CXXFLAGS="-m64 -O2" FCFLAGS="-m64 -O2" FFLAGS="-m64 -O2"';
                $openmpi_comp_env .= ' --with-wrapper-ldflags="-g -O2 -m64 -L/usr/lib64" --with-wrapper-cflags=-m64';
                $openmpi_comp_env .= ' --with-wrapper-cxxflags=-m64 --with-wrapper-fflags=-m64 --with-wrapper-fcflags=-m64';
                $openmpi_wrapper_cxx_flags .= " -m64";
            }

            if ($openmpi_wrapper_cxx_flags) {
                $openmpi_comp_env .= " --with-wrapper-cxxflags=\"$openmpi_wrapper_cxx_flags\"";
            }

            $openmpi_comp_env .= " --with-platform=contrib/platform/mellanox/optimized $pmi_opt";

            # Let openmpi automatically find plugins and use them
            $ENV{"mellanox_autodetect"} = "yes";

            $cmd .= " --define '_name $name'";
            $cmd .= " --define 'install_shell_scripts 1'";
            $cmd .= " --define 'shell_scripts_basename mpivars'";
            $cmd .= " --define '_usr $prefix'";
            $cmd .= " --define 'ofed 0'";
            $cmd .= " --define '_prefix $prefix/mpi/$compiler/$parent-$main_packages{$parent}{$ver}{'version'}'";
            $cmd .= " --define '_defaultdocdir $prefix/mpi/$compiler/$parent-$main_packages{$parent}{$ver}{'version'}/doc'";
            $cmd .= " --define '_mandir %{_prefix}/share/man'";
            $cmd .= " --define '_datadir %{_prefix}/share'";
            $cmd .= " --define 'mflags -j 4'";
            $cmd .= " --define 'configure_options $packages_info{'openmpi'}{'configure_options'} $openmpi_ldflags $openmpi_comp_env '";
            $cmd .= " --define 'use_default_rpm_opt_flags $use_default_rpm_opt_flags'";

            if ($DISTRO =~ /RHEL8 | OL8 | BCLINUX8 | EULEROS2.0SP[89] | OPENEULER/x) {
                $cmd .= " --undefine _hardened_build";
            }
            if ($DISTRO =~ /RHEL10/x) {
                $cmd .= " --define '_hardening_gcc_ldflags %{nil}'";
                $cmd .= " --define '__perl /usr/bin/perl'";
            }
        }
        elsif ($parent eq "mpi-selector") {
            $cmd .= " --define '_prefix $prefix'";
            $cmd .= " --define '_exec_prefix $prefix'";
            $cmd .= " --define '_sysconfdir $sysconfdir'";
            $cmd .= " --define '_usr $prefix'";
            $cmd .= " --define 'shell_startup_dir /etc/profile.d'";
        }
        elsif ($parent =~ m/dapl/) {
            my $def_doc_dir = `rpm --eval '%{_defaultdocdir}'`;
            chomp $def_doc_dir;
            $cmd .= " --define '_prefix $prefix'";
            $cmd .= " --define '_exec_prefix $prefix'";
            $cmd .= " --define '_sysconfdir $sysconfdir'";
            $cmd .= " --define '_defaultdocdir $def_doc_dir/$main_packages{$parent}{$ver}{'name'}-$main_packages{$parent}{$ver}{'version'}'";
            $cmd .= " --define '_usr $prefix'";
        }
        elsif ($parent =~ /fabric-collector/) {
            $cmd .= " --define '_prefix /opt/mellanox/fabric_collector'";
        }
        elsif ($parent =~ /mlnx-ethtool/) {
            $cmd .= " --define '_prefix $mlnx_ethtool_prefix'";
            $cmd .= " --define '_mandir $mlnx_ethtool_mandir'";
            $cmd .= " --define '_defaultdocdir $mlnx_ethtool_defaultdocdir'";
        }
        elsif ($parent eq 'rdma-core') {
            if ($libudev_devel eq "") {
                # FIXME: XenServer et al. don't have udev at all.
                # The rdma-core will build fine without it, but not
                # having it probably breaks an assumption or two in the
                # pacaging. Add an option to build the package --without
                # udev to rdma-core and replace this with '--without udev'
                $cmd .= " --nodeps";
            }
            if ($packages_info{'python3-pyverbs'}{'available'}) {
                # Due to rdma-core dependencies it requires with_pyverbs to be
                # enabled on RHEL8
                $cmd .= " --define '_with_pyverbs 1'";
            }
            if ($DISTRO =~ /RHEL6 | OL6/x) {
                $cmd .= " --without libnl";
            }
        }
        else {
            $cmd .= " --define '_prefix $prefix'";
            $cmd .= " --define '_exec_prefix $prefix'";
            $cmd .= " --define '_sysconfdir $sysconfdir'";
            $cmd .= " --define '_usr $prefix'";
        }

        if ($parent =~ /mstflint/) {
            $cmd .= " --define 'noinband 1'";
            my $openssl_version = `/usr/bin/openssl version 2> /dev/null | cut -d ' ' -f 2`;
            chomp $openssl_version;
            my $nopenssl = 1;
            if ($openssl_version =~ /^1.*/) {
                $nopenssl = 0;
            }
            $cmd .= " --define 'nopenssl $nopenssl'";
            $cmd .= " --define 'ibmadlib %{nil}'";
        }

        if ($parent eq "mft") {
            $cmd .= " --define 'nodevmon 1'";
        }

        if ($parent eq "librdmacm") {
            if ( $packages_info{'ibacm'}{'selected'}) {
                $packages_info{'librdmacm'}{'configure_options'} .= " --with-ib_acm";
            }
        }

        if ($parent eq "mlnx-iproute2") {
            if ($with_bluefield) {
                $cmd .= " --with bluefield ";
            }
        }

        if ($packages_info{$parent}{'configure_options'} or $user_configure_options) {
            $cmd .= " --define 'configure_options $packages_info{$parent}{'configure_options'} $user_configure_options'";
        }

        if ( $parent =~ m/libibverbs/ and ($rpm_distro =~ m/xenserver|sles10/) ) {
            $cmd .= " --define 'configure_options --without-resolve-neigh' ";
        }

        if ($parent =~ m/libibverbs|libmlx|librdmacm/) {
            if ($with_valgrind) {
                $cmd .= " --define '_with_valgrind 1'";
            } else {
                if (($DISTRO !~ m/RHEL/) or $disable_valgrind) {
                    $cmd .= " --define '_disable_valgrind 1'";
                }
            }
        }
        if ($parent =~ /ucx/) {
            if ($with_valgrind or (($DISTRO =~ m/RHEL/) and (not $disable_valgrind))) {
                $cmd .= " --with valgrind ";
            }
        }
        if ($parent =~ /ucx/) {
            if (not $have_numa_libs) {
                $cmd .= " --define 'configure_options --disable-numa' ";
            }
            $cmd .= " --with mlx5";
            $cmd .= " --nodeps ";
            $cmd .= " --without java";
            if ($cuda_top_dir) {
                $cmd .= " --with cuda";
                $cmd .= " --define 'configure_options --with-cuda=$cuda_top_dir'";
            }
            for my $sp (qw/gdrcopy knem xpmem/) {
                if ($packages_info{"ucx-$sp"}{'available'}) {
                    $cmd .= " --with $sp";
                }
            }
            if (not $packages_info{'ucx-cma'}{'available'}) {
                $cmd .= " --without cma";
            }
        }
        if ($parent =~ /openvswitch/) {
            $cmd .= " --without check ";
        }

        if ($parent =~ /mlnx-dpdk/) {
            if ($with_bluefield) {
                $cmd .= " --with bluefield";
            }
        }

        if ($parent =~ /mlxbf-bootctl/) {
            $cmd .= " --define 'set_build_flags %{nil}'";
        }

        my $munge_path_str = kernel_build_munge_path();
        $cmd .= " '$main_packages{$parent}{$ver}{'srpmpath'}'";

        if (exists $package_pre_build_script{$name}) {
            print_and_log("Running $name pre build script: $package_pre_build_script{$name}\n", $verbose);
            ex("$munge_path_str $package_pre_build_script{$name}");
        }

        print_and_log("Running $cmd\n", $verbose);
        open(LOG, "+>$ofedlogs/$parent-$ver.rpmbuild.log");
        print LOG "Running $cmd\n";
        close LOG;
        system("$cmd >> $ofedlogs/$parent-$ver.rpmbuild.log 2>&1");
        $res = $? >> 8;
        $sig = $? & 127;
        if ($sig or $res) {
            print_and_log_colored("Failed to build $parent $ver RPM", 1, "RED");
            addSetupInfo ("$ofedlogs/$parent-$ver.rpmbuild.log");
            print_and_log_colored("See $ofedlogs/$parent-$ver.rpmbuild.log", 1, "RED");
            exit 1;
        }

        $TMPRPMS = "$TOPDIR/RPMS/$target_cpu";
        chomp $TMPRPMS;
        my $NOARCHRPMS = "$TOPDIR/RPMS/noarch";

        print_and_log("TMPRPMS $TMPRPMS\n", $verbose2);

        my @built_rpms = ();
        for my $myrpm (<$TMPRPMS/*.rpm>, <$NOARCHRPMS/*rpm>) {
            print_and_log("Created $myrpm\n", $verbose2);
            my ($myrpm_name, $myrpm_arch) = (split ' ', get_rpm_name_arch($myrpm));
            move($myrpm, "$RPMS/");
            $packages_info{$myrpm_name}{$ver}{'rpm_exist'} = 1;
            my $built_rpm = $myrpm;
            $built_rpm =~ s@.*/@${RPMS}/@;
            push(@built_rpms, $built_rpm);
        }
        $packages_info{$name}{'built_rpms'} = \@built_rpms;
    }

    if (exists $package_post_build_script{$name}) {
        print_and_log("Running $name post build script: $package_post_build_script{$name}\n", $verbose);
        ex("$package_post_build_script{$name}");
    }
}

sub install_kernel_rpm
{
    my $name = shift @_;
    my $ver = shift @_;
    my $cmd;
    my $res = 0;
    my $sig = 0;
    my $rpm_distro = get_rpmbuild_dist(); # overrides global
    my $dist_part = $rpm_distro ? ".$rpm_distro" : "";

    my $version = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'version'};

    # release number is from source without kver
    my $release = "$main_packages{$packages_info{$name}{'parent'}}{$ver}{'release'}";
    $release =~ s/\.$rpm_distro$//g;
    $release = "${release}.kver.${kernel_rel}";

    my $arch = $target_cpu;

    if (exists $package_pre_install_script{$name}) {
        print_and_log("Running $name pre install script: $package_pre_install_script{$name}\n", $verbose);
        ex("$package_pre_install_script{$name}");
    }

    # will enter this block if  KMP is enabled and this is a package that supports kmp
    if (($name =~ /kernel-mft-mlnx|iser|srp|mlnx-en$|mlnx-nfsrdma|mlnx-nvme|mlnx-rdma-rxe|fwctl|virtiofs/ and $name !~ /-dkms/) and $kmp) {
        # WA required to get rpmpath for KMP RPMs
        set_existing_rpms();
        if ($DISTRO =~ /XenServer/ and $target_cpu eq "i386") {
            $arch = 'i686';
        }

        if ($DISTRO =~ /SLES/) {
            my $kver = get_rpm_ver_one('kernel-source');
            if ($name =~ /kmp/ and  $kver !~ /$kernel_rel/) {
                return;
            }

        }

        # KMP packages that have only kmod/kmp rpms
        if ($name !~ /kernel-mft-mlnx|iser|srp|mlnx-en|mlnx-nfsrdma|mlnx-nvme|mlnx-rdma-rxe|fwctl|virtiofs/) {
            $cmd = "rpm $rpminstall_parameter -vh $rpminstall_flags";
            $cmd .= " --nodeps";
            $cmd .= " '$main_packages{$name}{$ver}{'rpmpath'}'";

            print_and_log("Installing $name $ver RPM\n", $verbose);
            print_and_log("cmd: $cmd\n", $verbose);
            print_and_log("Running $cmd\n", $verbose2);
            system("$cmd > $ofedlogs/$name-$ver.rpminstall.log 2>&1");
            $res = $? >> 8;
            $sig = $? & 127;
            if ($sig or $res) {
                print_and_log_colored("Failed to install $name $ver RPM", 1, "RED");
                addSetupInfo ("$ofedlogs/$name-$ver.rpminstall.log");
                print_and_log("See $ofedlogs/$name-$ver.rpminstall.log", 1, "RED");
                exit $ERROR;
            }
            system("cat $ofedlogs/$name-$ver.rpminstall.log") if (not $quiet);
            system("/sbin/depmod $kernel > /dev/null 2>&1");

            if ($name =~ /mlnx-ofa_kernel-(devel|source)/) {
                if (exists $package_post_install_script{$name}) {
                    print_and_log("Running $name post install script: $package_post_install_script{$name}\n", $verbose);
                    ex("$package_post_install_script{$name}");
                }
                return;
            }
        }

        my @kmp_packages;
        if ($DISTRO =~ /SLES|SUSE/) {
            @kmp_packages = <$RPMS/$name-kmp*$dist_part.$arch.rpm>;
        } else {
            @kmp_packages = <$RPMS/kmod*$name-*$dist_part.$arch.rpm>;
        }

        if (not @kmp_packages) {
            print_and_log_colored("$name KMP rpms do not exist", 1, "RED");
            exit $ERROR;
        }

        # Install kernel-mft-mlnx-utils when available - relevant only when KMP=1
        if ($name =~ /kernel-mft-mlnx/) {
            my @mft_utils = <$RPMS/$name-utils*$dist_part.$arch.rpm>;
            if (@mft_utils) {
                push(@kmp_packages, $mft_utils[0]);
            }
        }

        for my $kmp_package (@kmp_packages) {
            $cmd = "rpm $rpminstall_parameter -vh $rpminstall_flags";
            $cmd .= " --nodeps";
            if ($speed_up_kmp) {
                $cmd .= " --nopost";
                system("rpm -qpl '$kmp_package' | grep '\\.ko\$' >> '$modules_list'");
            }
            $cmd .= " '$kmp_package'";

            my $kmpname =`rpm -qp --queryformat "[%{NAME}]\n" '$kmp_package' 2>/dev/null`;
            chomp $kmpname;
            print_and_log("Installing $kmpname $ver RPM\n", $verbose);
            print_and_log("Running $cmd\n", $verbose2);
            system("$cmd > $ofedlogs/$kmpname-$ver.rpminstall.log 2>&1");
            $res = $? >> 8;
            $sig = $? & 127;
            if ($sig or $res) {
                print_and_log_colored("Failed to install $name $ver RPM", 1, "RED");
                addSetupInfo ("$ofedlogs/$kmpname-$ver.rpminstall.log");
                print_and_log_colored("See $ofedlogs/$kmpname-$ver.rpminstall.log", 1, "RED");
                exit $ERROR;
            }
            system("cat $ofedlogs/$kmpname-$ver.rpminstall.log") if (not $quiet);
            system("/sbin/depmod $kernel > /dev/null 2>&1");

            if (exists $package_post_install_script{$kmpname}) {
                print_and_log("Running $kmpname post install script: $package_post_install_script{$kmpname}\n", $verbose);
                ex("$package_post_install_script{$kmpname}");
            }
        }
        return;
    }

    if ((
          $name =~ /mlnx-ofa_kernel/ and not (
            ($name eq "mlnx-ofa_kernel-devel" and not $kmp) or
            ($name eq "mlnx-ofa_kernel-modules")
          )
        ) or
        ($name =~ /-dkms/) or
        ($name =~ /mlnx-en/) or
        ($name =~ /knem/ and $name ne "knem-modules")) {
        $release = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'release'} . $dist_part;
    }
    if ($name eq 'xpmem' or $name =~ /libxpmem|xpmem-dkms/) {
        $release = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'release'} . $dist_part;
    }
    if ($name eq "xpmem-modules") {
        $release =~ s/\.kver/$dist_part.kver/ if $rpm_distro;
    }

    if ($name =~ /mlnx-ofa_kernel/ and $DISTRO =~ /XenServer/ and $target_cpu eq "i386") {
        $arch = 'i686';
    }
    my $package = "$RPMS/$name-$version-$release.$arch.rpm";

    if (not -e $package) {
        my $rpmrel_nodist = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'release'};
        $rpmrel_nodist =~ s/\.$rpm_distro\$//g;
        my $removed_suffix_file = $package;
        $removed_suffix_file =~ s/(\.$arch)\.ky10(\.$arch\.rpm)$/$1$2/;
        $removed_suffix_file =~ s/(\.$arch)\.ctl2(\.$arch\.rpm)$/$1$2/;
        $removed_suffix_file =~ s/(\.$arch)\.ctl3(\.$arch\.rpm)$/$1$2/;
        $removed_suffix_file =~ s/(\.$arch)\.uel20(\.$arch\.rpm)$/$1$2/;
        $removed_suffix_file =~ s/(\.$arch)\.uelc20(\.$arch\.rpm)$/$1$2/;
        $removed_suffix_file =~ s/(\.$arch)\.oe1\.bclinux(\.$arch\.rpm)$/$1$2/;
        $removed_suffix_file =~ s/\.al8(\.$arch\.rpm)$/$1/;
        $removed_suffix_file =~ s/\.tl3(\.$arch\.rpm)$/$1/;
        $removed_suffix_file =~ s/\.an8(\.$arch\.rpm)$/$1/;
        $removed_suffix_file =~ s/\.azl3(\.$arch\.rpm)$/$1/;
        $removed_suffix_file =~ s/\.el9(\.$arch\.rpm)$/$1/;
        $removed_suffix_file =~ s/\.el10(\.$arch\.rpm)$/$1/;
        $removed_suffix_file =~ s/\.el8(\.$arch\.rpm)$/$1/;
        $removed_suffix_file =~ s/\.fc32(\.$arch\.rpm)$/$1/;
        $removed_suffix_file =~ s/\.amzn2023(\.$arch\.rpm)$/$1/;

        if (-e "$RPMS/$name-$version-$kernel_rel.$arch.rpm") {
            $package = "$RPMS/$name-$version-$kernel_rel.$arch.rpm";
        } elsif (-e "$RPMS/$name-$version-${kernel_rel}_$rpmrel_nodist.$arch.rpm") {
            $package = "$RPMS/$name-$version-${kernel_rel}_$rpmrel_nodist.$arch.rpm";
        } elsif (-e $removed_suffix_file) {
            print_and_log("Trimming Kylin suffix (kernel) for: $name\n", $verbose2);
            $package = $removed_suffix_file;
        } elsif (-e "$RPMS/$name-$version-$rpmrel_nodist.$arch.rpm") {
            $package = "$RPMS/$name-$version-$rpmrel_nodist.$arch.rpm";
        } else {
            print_and_log_colored("$package does not exist", 1, "RED");
            exit $ERROR;
        }
    }

    $cmd = "rpm -iv $rpminstall_flags";
    $cmd .= " --nodeps";
    $cmd .= " '$package'";

    print_and_log("Running $cmd\n", $verbose);
    system("$cmd > $ofedlogs/$name-$ver.rpminstall.log 2>&1");
    $res = $? >> 8;
    $sig = $? & 127;
    if ($sig or $res) {
        print_and_log_colored("Failed to install $name $ver RPM", 1, "RED");
        addSetupInfo ("$ofedlogs/$name-$ver.rpminstall.log");
        print_and_log_colored("See $ofedlogs/$name-$ver.rpminstall.log", 1, "RED");
        exit $ERROR;
    }

    if (exists $package_post_install_script{$name}) {
        print_and_log("Running $name post install script: $package_post_install_script{$name}\n", $verbose);
        ex("$package_post_install_script{$name}");
    }
}

# Install required RPM
sub install_rpm
{
    my $name = shift @_;
    my $ver = shift @_;
    my $tmp_name;
    my $cmd;
    my $res = 0;
    my $sig = 0;
    my $package;

    my $version = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'version'};
    my $release = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'release'};
    my $arch = $target_cpu;
    if ($packages_info{$name}{$ver}{'is_noarch'}) {
        $arch = 'noarch';
    }

    # user-space-only will get here
    if ($name eq "mlnx-ofa_kernel") {
        $release = $main_packages{$packages_info{$name}{'parent'}}{$ver}{'release'} . ".$rpm_distro";
    }

    $package = "$RPMS/$name-$version-$release.$arch.rpm";
    my $removed_suffix_file = $package;
    $removed_suffix_file =~ s/(\.$arch)\.ky10(\.$arch\.rpm)$/$1$2/;
    $removed_suffix_file =~ s/\.tencent(\.$arch\.rpm)$/$1$2/;
    if (($removed_suffix_file ne $package) and (-e $removed_suffix_file)) {
        print_and_log("Trimming Kylin/Tencent suffix (user) for: $name\n", $verbose2);
        $package = $removed_suffix_file;
    }
    if (not -e $package) {
        my $dist = get_rpmbuild_dist();
        my $with_dist_file = $package;
        $with_dist_file =~ s/\.$arch\.rpm$/\.$dist$&/;
        if (-e $with_dist_file) {
            print_and_log("Adding dist suffix $dist for: $name\n", $verbose2);
            $package = $with_dist_file;
        } else {
            print_and_log_colored("$package does not exist", 1, "RED");
            exit $ERROR;
        }
    }

    if ($name eq "mpi-selector") {
        $cmd = "rpm -Uv $rpminstall_flags --force";
    } elsif ($name =~ /openmpi/) {
        if (is_installed("$name-$version")) {
            system("rpm -e --nodeps $name-$version");
        }
        $cmd = "rpm -iv $rpminstall_flags --force";
    } else {
        $cmd = "rpm $rpminstall_parameter -v $rpminstall_flags";
    }

    $cmd .= " --nodeps";

    if ($name =~ /ibacm/) {
        $cmd .= " --noscripts";
    }

    $cmd .= " '$package'";

    if (exists $package_pre_install_script{$name}) {
        print_and_log("Running $name pre install script: $package_pre_install_script{$name}\n", $verbose);
        ex("$package_pre_install_script{$name}");
    }

    print_and_log("Running $cmd\n", $verbose);
    system("$cmd > $ofedlogs/$name-$ver.rpminstall.log 2>&1");
    $res = $? >> 8;
    $sig = $? & 127;
    if ($sig or $res) {
        print_and_log_colored("Failed to install $name $ver RPM", 1, "RED");
        addSetupInfo ("$ofedlogs/$name-$ver.rpminstall.log");
        print_and_log_colored("See $ofedlogs/$name-$ver.rpminstall.log", 1, "RED");
        exit $ERROR;
    }

    if (exists $package_post_install_script{$name}) {
        print_and_log("Running $name post install script: $package_post_install_script{$name}\n", $verbose);
        ex("$package_post_install_script{$name}");
    }
}

sub print_package_info
{
    print_and_log("\n\nDate:" . localtime(time) . "\n", 1);
    for my $key ( keys %main_packages ) {
        print_and_log("$key:\n", 1);
        print_and_log("======================================\n", 1);
        for my $ver ( keys %{$main_packages{$key}} ) {
            print_and_log("-------------------------------\n", 1);
            my %pack = %{$main_packages{$key}{$ver}};
            for my $subkey ( keys %pack ) {
                print_and_log($subkey . ' = ' . $pack{$subkey} . "\n", 1);
            }
        }
        print_and_log("\n", 1);
    }
}

sub is_installed
{
    my $res = 0;
    my $name = shift @_;

    system("rpm -q $name > /dev/null 2>&1");
    $res = $? >> 8;

    # cmake versions in any recent distribution is sane. Let's start
    # obsoleting it. Mark some distributions to not look for cmake
    # in the PATH:
    my $cmake_hack_required = ($DISTRO !~ /RHEL8 | OL8 | SLES15 | FC3./x);
    if($res and $cmake_hack_required and $name eq "cmake") {
        system("which cmake > /dev/null 2>&1");
        $res = $? >> 8;
    }

    return not $res;
}

sub count_ports
{
    my $cnt = 0;
    open(LSPCI, "/sbin/lspci -n|");

    while (<LSPCI>) {
        if (/15b3:6282/) {
            $cnt += 2;  # infinihost iii ex mode
        }
        elsif (/15b3:5e8c|15b3:6274/) {
            $cnt ++;    # infinihost iii lx mode
        }
        elsif (/15b3:5a44|15b3:6278/) {
            $cnt += 2;  # infinihost mode
        }
        elsif (/15b3:6340|15b3:634a|15b3:6354|15b3:6732|15b3:673c|15b3:6746|15b3:6750|15b3:1003/) {
            $cnt += 2;  # connectx
        }
    }
    close (LSPCI);

    return $cnt;
}

sub is_valid_ipv4
{
    my $ipaddr = shift @_;

    if( $ipaddr =~ m/^(\d\d?\d?)\.(\d\d?\d?)\.(\d\d?\d?)\.(\d\d?\d?)/ ) {
        if($1 <= 255 && $2 <= 255 && $3 <= 255 && $4 <= 255) {
            return 0;
        }
    }
    return 1;
}

sub is_carrier
{
    my $ifcheck = shift @_;
    open(IFSTATUS, "ip link show dev $ifcheck |");
    while ( <IFSTATUS> ) {
        next unless m@(\s$ifcheck).*@;
        if( m/NO-CARRIER/ or not m/UP/ ) {
            close(IFSTATUS);
            return 0;
        }
    }
    close(IFSTATUS);
    return 1;
}

sub config_interface
{
    my $interface = shift @_;
    my $ans;
    my $dev = "ib$interface";
    my $target = "$network_dir/ifcfg-$dev";
    my $ret;
    my $ip;
    my $nm;
    my $nw;
    my $bc;
    my $onboot = 1;
    my $found_eth_up = 0;

    if ($interactive) {
        print "\nDo you want to configure $dev? [Y/n]:";
        $ans = getch();
        if ($ans =~ m/[nN]/) {
            return;
        }
        if (-e $target) {
            print "\nThe current IPoIB configuration for $dev is:\n";
            open(IF,$target);
            while (<IF>) {
                print $_;
            }
            close(IF);
            print "\nDo you want to change this configuration? [y/N]:";
            $ans = getch();
            if ($ans !~ m/[yY]/) {
                return;
            }
        }
        print "\nEnter an IP Adress: ";
        $ip = <STDIN>;
        chomp $ip;
        $ret = is_valid_ipv4($ip);
        while ($ret) {
            print "\nEnter a valid IPv4 Adress: ";
            $ip = <STDIN>;
            chomp $ip;
            $ret = is_valid_ipv4($ip);
        }
        print "\nEnter the Netmask: ";
        $nm = <STDIN>;
        chomp $nm;
        $ret = is_valid_ipv4($nm);
        while ($ret) {
            print "\nEnter a valid Netmask: ";
            $nm = <STDIN>;
            chomp $nm;
            $ret = is_valid_ipv4($nm);
        }
        print "\nEnter the Network: ";
        $nw = <STDIN>;
        chomp $nw;
        $ret = is_valid_ipv4($nw);
        while ($ret) {
            print "\nEnter a valid Network: ";
            $nw = <STDIN>;
            chomp $nw;
            $ret = is_valid_ipv4($nw);
        }
        print "\nEnter the Broadcast Adress: ";
        $bc = <STDIN>;
        chomp $bc;
        $ret = is_valid_ipv4($bc);
        while ($ret) {
            print "\nEnter a valid Broadcast Adress: ";
            $bc = <STDIN>;
            chomp $bc;
            $ret = is_valid_ipv4($bc);
        }
        print "\nStart Device On Boot? [Y/n]:";
        $ans = getch();
        if ($ans =~ m/[nN]/) {
            $onboot = 0;
        }

        print GREEN "\nSelected configuration:\n";
        print "DEVICE=$dev\n";
        print "IPADDR=$ip\n";
        print "NETMASK=$nm\n";
        print "NETWORK=$nw\n";
        print "BROADCAST=$bc\n";
        if ($DISTRO =~ /RHEL6|RHEL7|POWERKVM|EULEROS2.0SP[235]|BCLINUX7|ALIOS/) {
            print "NM_CONTROLLED=yes\n";
            print "TYPE=InfiniBand\n";
        }
        if ($onboot) {
            print "ONBOOT=yes\n";
        }
        else {
            print "ONBOOT=no\n";
        }
        print "\nDo you want to save the selected configuration? [Y/n]:";
        $ans = getch();
        if ($ans =~ m/[nN]/) {
            return;
        }
    }
    else {
        if (not $config_net_given) {
            return;
        }
        print "Going to update $target\n" if ($verbose2);
        if ($ifcfg{$dev}{'LAN_INTERFACE'}) {
            $eth_dev = $ifcfg{$dev}{'LAN_INTERFACE'};
            if (not -e "/sys/class/net/$eth_dev") {
                print "Device $eth_dev is not present\n" if (not $quiet);
                return;
            }
            if ( is_carrier ($eth_dev) ) {
                $found_eth_up = 1;
            }
        }
        else {
            # Take the first existing Eth interface
            my @eth_devs = </sys/class/net/eth*>;
            for my $tmp_dev ( @eth_devs ) {
                $eth_dev = $tmp_dev;
                $eth_dev =~ s@/sys/class/net/@@g;
                if ( is_carrier ($eth_dev) ) {
                    $found_eth_up = 1;
                    last;
                }
            }
        }

        if ($found_eth_up) {
            get_net_config($eth_dev, \%ifcfg, $DISTRO);
        }

        if (not $ifcfg{$dev}{'IPADDR'}) {
            print "IP address is not defined for $dev\n" if ($verbose2);
            print "Skipping $dev configuration...\n" if ($verbose2);
            return;
        }
        if (not $ifcfg{$dev}{'NETMASK'}) {
            print "Netmask is not defined for $dev\n" if ($verbose2);
            print "Skipping $dev configuration...\n" if ($verbose2);
            return;
        }
        if (not $ifcfg{$dev}{'NETWORK'}) {
            print "Network is not defined for $dev\n" if ($verbose2);
            print "Skipping $dev configuration...\n" if ($verbose2);
            return;
        }
        if (not $ifcfg{$dev}{'BROADCAST'}) {
            print "Broadcast address is not defined for $dev\n" if ($verbose2);
            print "Skipping $dev configuration...\n" if ($verbose2);
            return;
        }

        my @ipib = (split('\.', $ifcfg{$dev}{'IPADDR'}));
        my @nmib = (split('\.', $ifcfg{$dev}{'NETMASK'}));
        my @nwib = (split('\.', $ifcfg{$dev}{'NETWORK'}));
        my @bcib = (split('\.', $ifcfg{$dev}{'BROADCAST'}));

        my @ipeth = (split('\.', $ifcfg{$eth_dev}{'IPADDR'}));
        my @nmeth = (split('\.', $ifcfg{$eth_dev}{'NETMASK'}));
        my @nweth = (split('\.', $ifcfg{$eth_dev}{'NETWORK'}));
        my @bceth = (split('\.', $ifcfg{$eth_dev}{'BROADCAST'}));

        for (my $i = 0; $i < 4 ; $i ++) {
            if ($ipib[$i] =~ m/\*/) {
                if ($ipeth[$i] =~ m/(\d\d?\d?)/) {
                    $ipib[$i] = $ipeth[$i];
                }
                else {
                    print "Cannot determine the IP address of the $dev interface\n" if (not $quiet);
                    return;
                }
            }
            if ($nmib[$i] =~ m/\*/) {
                if ($nmeth[$i] =~ m/(\d\d?\d?)/) {
                    $nmib[$i] = $nmeth[$i];
                }
                else {
                    print "Cannot determine the netmask of the $dev interface\n" if (not $quiet);
                    return;
                }
            }
            if ($bcib[$i] =~ m/\*/) {
                if ($bceth[$i] =~ m/(\d\d?\d?)/) {
                    $bcib[$i] = $bceth[$i];
                }
                else {
                    print "Cannot determine the broadcast address of the $dev interface\n" if (not $quiet);
                    return;
                }
            }
            if ($nwib[$i] !~ m/(\d\d?\d?)/) {
                $nwib[$i] = $nweth[$i];
            }
        }

        $ip = "$ipib[0].$ipib[1].$ipib[2].$ipib[3]";
        $nm = "$nmib[0].$nmib[1].$nmib[2].$nmib[3]";
        $nw = "$nwib[0].$nwib[1].$nwib[2].$nwib[3]";
        $bc = "$bcib[0].$bcib[1].$bcib[2].$bcib[3]";

        print GREEN "IPoIB configuration for $dev\n";
        print "DEVICE=$dev\n";
        print "IPADDR=$ip\n";
        print "NETMASK=$nm\n";
        print "NETWORK=$nw\n";
        print "BROADCAST=$bc\n";
        if ($onboot) {
            print "ONBOOT=yes\n";
        }
        else {
            print "ONBOOT=no\n";
        }
        print RESET "\n";
    }

    open(IF, ">$target") or die "Can't open $target: $!";
    if ($DISTRO =~ m/SLES|SUSE/) {
        print IF "BOOTPROTO='static'\n";
        print IF "IPADDR='$ip'\n";
        print IF "NETMASK='$nm'\n";
        print IF "NETWORK='$nw'\n";
        print IF "BROADCAST='$bc'\n";
        print IF "REMOTE_IPADDR=''\n";
        if ($onboot) {
            print IF "STARTMODE='onboot'\n";
        }
        else {
            print IF "STARTMODE='manual'\n";
        }
        print IF "WIRELESS=''\n";
    }
    else {
        print IF "DEVICE=$dev\n";
        print IF "BOOTPROTO=static\n";
        print IF "IPADDR=$ip\n";
        print IF "NETMASK=$nm\n";
        print IF "NETWORK=$nw\n";
        print IF "BROADCAST=$bc\n";
        if ($DISTRO =~ /RHEL6|RHEL7|POWERKVM|EULEROS2.0SP[235]|BCLINUX7|ALIOS/) {
            print IF "NM_CONTROLLED=yes\n";
            print IF "TYPE=InfiniBand\n";
        }
        if ($onboot) {
            print IF "ONBOOT=yes\n";
        }
        else {
            print IF "ONBOOT=no\n";
        }
    }
    close(IF);
}

sub ipoib_config
{
    if ($interactive) {
        print "\nThe default IPoIB interface configuration is based on DHCP.";
        print "\nNote that a special patch for DHCP is required for supporting IPoIB.";
        print "\nThe patch is available under docs/dhcp";
        print "\nIf you do not have DHCP, you must change this configuration in the following steps.\n";
    }

    my $ports_num = count_ports();
    for (my $i = 0; $i < $ports_num; $i++ ) {
        config_interface($i);
    }

    if ($interactive) {
        print GREEN "IPoIB interfaces configured successfully",RESET "\n";
        print "Press any key to continue ...";
        getch();
    }

    if (-e "/etc/sysconfig/network/config") {
        my $nm = `grep ^NETWORKMANAGER=yes /etc/sysconfig/network/config`;
        chomp $nm;
        if ($nm) {
            print RED "Please set NETWORKMANAGER=no in the /etc/sysconfig/network/config", RESET "\n";
        }
    }

}

sub is_included($) {
    my $package = shift;
    return exists $main_packages{$package};
}

sub get_uninstall_env_str() {
    my $components_str = 'MLNX_OFED_COMPONENTS="'. join(" ", @components) . '"';
    my $keep_packages_regex = 'openvswitch';
    if (not is_included('spdk')) {
        $keep_packages_regex .= '|spdk';
    }
    my $keep_packages_str = "MLNX_OFED_KEEP_PACKAGES_REGEX=\"$keep_packages_regex\"";
    return "$components_str $keep_packages_str";
}
sub uninstall_mlnx_en
{
    my $res = 0;
    my $sig = 0;
    my $cnt = 0;

    if ( -e "/sbin/mlnx_en_uninstall.sh" ) {
        print_and_log("Uninstalling MLNX_EN driver\n", (not $quiet));
        my $env_str = get_uninstall_env_str();
        system("yes | $env_str /sbin/mlnx_en_uninstall.sh > $ofedlogs/mlnx_en_uninstall.log 2>&1");
        $res = $? >> 8;
        $sig = $? & 127;
        if ($sig or $res) {
            print_and_log_colored("Failed to uninstall MLNX_EN driver", 1, "RED");
            addSetupInfo ("$ofedlogs/mlnx_en_uninstall.log");
            print_and_log_colored("See $ofedlogs/mlnx_en_uninstall.log", 1, "RED");
            exit $ERROR;
        }
    }

    my $mlnx_en_cnt = 0;
    my $mlnx_en_rpms;
	# grep mlnx is required as a w/a on WR OS
    my @other_mlnx_en_rpms = `rpm -qa --queryformat "[%{NAME}]\n" 2> /dev/null | grep -E 'mlnx.en|mlx.*en' | grep -v '^kernel-module'`;
    for my $package (@mlnx_en_packages, @other_mlnx_en_rpms) {
        chomp $package;
        if ((is_installed $package) and $components_filter->($package)) {
            $mlnx_en_rpms .= " $package";
            $mlnx_en_cnt ++;
        }
    }

    if ($mlnx_en_cnt) {
            my $cmd = "rpm -e --allmatches --nodeps";
            $cmd .= " $mlnx_en_rpms";
            print_and_log("Uninstalling MLNX_EN driver\n", (not $quiet));
            system("$cmd >> $ofedlogs/mlnx_en_uninstall.log 2>&1");
            $res = $? >> 8;
            $sig = $? & 127;
            if ($sig or $res) {
                print_and_log_colored("Failed to uninstall MLNX_EN driver", 1, "RED");
                addSetupInfo ("$ofedlogs/mlnx_en_uninstall.log");
                print_and_log("See $ofedlogs/mlnx_en_uninstall.log", 1, "RED");
                exit $ERROR;
            }
    }

}

sub uninstall_mft
{
    my $res = 0;
    my $sig = 0;
    my $cnt = 0;

    my $mft_rpms;
    for my $package (@mft_packages) {
        chomp $package;
        if (is_installed($package)) {
            $mft_rpms .= " $package";
            if (not $selected_for_uninstall{$package}) {
                push (@packages_to_uninstall, $package);
                $selected_for_uninstall{$package} = 1;
            }
        }
    }

    if (open (KMP_RPMS, 'rpm -qa --queryformat "[%{NAME}]\n" *kernel-mft* 2>/dev/null | grep kernel-mft |')) {
        my $kmp_cnt = 0;
        my $kmp_rpms;
        while(<KMP_RPMS>) {
            chomp $_;
            $kmp_rpms .= " $_";
            $kmp_cnt ++;
        }
        close KMP_RPMS;

	    if ($kmp_cnt) {
            system("rpm -e --allmatches --nodeps $kmp_rpms >> $ofedlogs/kmp_rpms_uninstall.log 2>&1");
            $res = $? >> 8;
            $sig = $? & 127;
            if ($sig or $res) {
                print_and_log("Failed to uninstall kernel-mft-mlnx KMP RPMs", 1, "RED");
                exit $ERROR;
            }
        }
    }
}

# Try to remove a package, if failed, retry with adding --noscripts flag
# Use it as W/A for known possible problematic packages
sub remove_od_noscripts
{
    my $pname = shift;

    return unless (is_installed($pname));
    my $cmd = "rpm -e --allmatches --nodeps $pname";
    print_and_log("\n$cmd\n", (not $quiet));
    open (LOG, "+>$ofedlogs/ofed_uninstall.log");
    print LOG "$cmd\n";
    close LOG;
    system("$cmd >> $ofedlogs/ofed_uninstall.log 2>&1");
    my $res = $? >> 8;
    my $sig = $? & 127;
    if ($sig or $res) {
        $cmd = "rpm -e --allmatches --nodeps --noscripts $pname";
        print_and_log("\n$cmd\n", (not $quiet));
        system("$cmd >> $ofedlogs/ofed_uninstall.log 2>&1");
        $res = $? >> 8;
        $sig = $? & 127;
        if ($sig or $res) {
            print_and_log_colored("Failed to uninstall $pname", 1, "RED");
            addSetupInfo ("$ofedlogs/ofed_uninstall.log");
            print_and_log_colored("See $ofedlogs/ofed_uninstall.log", 1, "RED");
            exit $ERROR;
        }
    }
}

sub force_uninstall
{
    my $res = 0;
    my $sig = 0;
    my $cnt = 0;
    my @other_ofed_rpms = `rpm -qa --queryformat "[%{NAME}]\n" 2> /dev/null | grep -wE "rdma|ofed|openib" | grep -vEw "glusterfs|kernel-module|^xen|ovsvf-config|opensmtpd"`;
    my $cmd = "rpm -e --allmatches --nodeps";

    my @to_remove_packages = (@all_packages, @hidden_packages, @prev_ofed_packages, @other_ofed_rpms, @distro_ofed_packages, @dpdk_packages);
    if (not is_included('spdk')) {
        @to_remove_packages = grep !/^spdk/, @to_remove_packages;
    }
    for my $package (@to_remove_packages) {
        chomp $package;
        next if ($package eq "mpi-selector" or $package =~ /^kernel-module|glusterfs|^xen|ovsvf-config|opensmtpd/);
        if ((is_installed $package) and $components_filter->($package)) {
            push (@packages_to_uninstall, $package);
            $selected_for_uninstall{$package} = 1;
        }
        if (is_installed("$package-static")) {
            push (@packages_to_uninstall, "$package-static");
            $selected_for_uninstall{$package} = 1;
        }
    }

    for my $package (@packages_to_uninstall) {
        get_requires($package);
    }

    if (not $force and keys %non_ofed_for_uninstall) {
        print_and_log("\nError: One or more packages depends on MLNX_OFED.\nThose packages should be removed before uninstalling MLNX_OFED:\n\n", 1);
        print_and_log(join(" ", (keys %non_ofed_for_uninstall)) . "\n\n", 1);
        print_and_log("To force uninstallation use '--force' flag.\n", 1);
        exit $NONOFEDRPMS;
    }

    for my $package (@packages_to_uninstall, @dependant_packages_to_uninstall) {
        next if ($package =~ /pcp-pmda-infiniband/);
        if ("$package" eq "ofed" and is_installed("ofed")) {
            # W/A for SLES 10 SP4 in-box ofed RPM uninstall issue
            remove_od_noscripts("ofed");
            next;
        }
        if (is_installed("$package")) {
            $cmd .= " $package";
            $cnt ++;
        }
    }

    if ($cnt) {
        print_and_log("\n$cmd\n", (not $quiet));
        open (LOG, "+>$ofedlogs/ofed_uninstall.log");
        print LOG "$cmd\n";
        close LOG;
        system("$cmd >> $ofedlogs/ofed_uninstall.log 2>&1");
        $res = $? >> 8;
        $sig = $? & 127;
        if ($sig or $res) {
            print_and_log_colored("Failed to uninstall the previous installation", 1, "RED");
            addSetupInfo ("$ofedlogs/ofed_uninstall.log");
            print_and_log_colored("See $ofedlogs/ofed_uninstall.log", 1, "RED");
            exit $ERROR;
        }
    }
}

# if we install the drivers without KMP support, and then installed a new kernel
# it might still create weak-updates symlinks to our modules, and once we remove
# our rpms, no one will remove those links since our rpms do not know to run
# the systems tools that are responsible for removing the links.
# Note: if we have ofed kmp rpms installed, then we don't need to do anything here.
sub remove_weak_updates_symlinks
{
    my $ofed_kpkgs = "";
    my $got_kmps = 0;

    if ($DISTRO !~ /RHEL/) {
        return;
    }

    # get list of installed packages
    for my $package (qw(srp iser ofa_kernel knem ummunotify mlnx-sdp mlnx-rds mlnx-nfsrdma mlnx-nvme fwctl virtiofs mlnx-rdma-rxe kernel-mft gpio-mlxbf i2c-mlx mlxbf-livefish mlx-bootctl pka-mlxbf tmfifo mlxbf-gige mlx-trio gpio-mlxbf2 mlx-pmc)) {
        my $regEx = ".*$package*";
        if (open (OFED_KRPMS, "rpm -qa --queryformat \"[%{NAME}]\n\" $regEx | grep -E $regEx |")) {
            while(<OFED_KRPMS>) {
                chomp $_;
                if ($_ =~ /kmp|kmod/) {
                    $got_kmps = 1;
                    last;
                }
                $ofed_kpkgs .= " $_";
            }
            close OFED_KRPMS;
            if ($got_kmps) {
                last;
            }
        }
    }
    if ($got_kmps or $ofed_kpkgs eq "") {
        return;
    }

    # get list of KO files
    my $tmpfile = "/var/run/rpm_ofed_kernel_modules";
    system("rpm -ql $ofed_kpkgs | grep '\.ko\$' > $tmpfile");
    if (-z $tmpfile) {
        system("/bin/rm -f $tmpfile >/dev/null 2>&1");
        return;
    }

    # remove existing symlinks
    my $wm_tool = "/sbin/weak-modules";
    if (-e "$wm_tool") {
        system("cat $tmpfile | $wm_tool --remove-modules --no-initramfs --delete-modules >/dev/null 2>&1");
    }

    system("/bin/rm -f $tmpfile >/dev/null 2>&1");
}

sub uninstall
{
    my $res = 0;
    my $sig = 0;
    my $distro_rpms = '';

    return 0 if (not $uninstall);

    remove_weak_updates_symlinks();

    uninstall_mlnx_en();

    uninstall_mft();

    my $ofed_uninstall = `which ofed_uninstall.sh 2> /dev/null`;
    chomp $ofed_uninstall;
    if (-e "$ofed_uninstall") {
        my $env_str = get_uninstall_env_str();
        print_and_log("Uninstalling the previous version of $PACKAGE\n", (not $quiet));
        if ($force) {
                system("yes | $env_str ofed_uninstall.sh --force >> $ofedlogs/ofed_uninstall.log 2>&1");
        } else {
                system("yes | $env_str ofed_uninstall.sh >> $ofedlogs/ofed_uninstall.log 2>&1");
        }
        $res = $? >> 8;
        $sig = $? & 127;
        if ($sig or $res) {
            if ($res == 174) {
                print_and_log("Error: One or more packages depends on MLNX_OFED.\nThese packages should be removed before uninstalling MLNX_OFED:\n", 1);
                system("cat $ofedlogs/ofed_uninstall.log | perl -ne '/Those packages should be/ && do {\$a=1; next}; /To force uninstallation use/ && do {\$a=0}; print if \$a'");
                print_and_log("To force uninstallation use '--force' flag.\n", (not $quiet));
                addSetupInfo ("$ofedlogs/ofed_uninstall.log");
                print_and_log_colored("See $ofedlogs/ofed_uninstall.log", 1, "RED");
                exit $NONOFEDRPMS;
            }
            print_and_log_colored("Failed to uninstall the previous installation", 1, "RED");
            addSetupInfo ("$ofedlogs/ofed_uninstall.log");
            print_and_log_colored("See $ofedlogs/ofed_uninstall.log", 1, "RED");
            exit $ERROR;
        }
    }

    # Uninstall leftovers and previous OFED packages
    force_uninstall();

    # uninstall KMP packages
    for my $package (qw(srp iser isert ofa_kernel knem xpmem ummunotify mlnx-sdp mlnx-rds mlnx-nfsrdma mlnx-nvme fwctl virtiofs mlnx-rdma-rxe gpio-mlxbf i2c-mlx mlxbf-livefish mlx-bootctl pka-mlxbf tmfifo mlxbf-gige mlx-trio gpio-mlxbf2 mlx-pmc)) {
        last if ($user_space_only);
        my $regEx;
        if ($package !~ /iser|srp$/) {
            $regEx = ".*$package*";
        } else {
            $regEx = "\"kmod-$package*|$package-kmp*\"";
        }
        if (open (KMP_RPMS, "rpm -qa --queryformat \"[%{NAME}]\n\" $regEx 2>/dev/null | grep -E $regEx |")) {
            my $kmp_cnt = 0;
            my $kmp_rpms;
            while(<KMP_RPMS>) {
                chomp $_;
                next if ($_ eq "mlnx-ofa_kernel");
                $kmp_rpms .= " $_";
                $kmp_cnt ++;
            }
            close KMP_RPMS;

            if ($kmp_cnt) {
                if ($package eq "ofa_kernel") {
                    remove_od_noscripts("mlnx-ofa_kernel");
                }
                print_and_log("rpm -e --allmatches --nodeps $kmp_rpms\n", $verbose);
                system("rpm -e --allmatches --nodeps $kmp_rpms >> $ofedlogs/kmp_$package\_rpms_uninstall.log 2>&1");
                $res = $? >> 8;
                $sig = $? & 127;
                if ($sig or $res) {
                    print_and_log_colored("Failed to uninstall $package KMP RPMs", 1, "RED");
                    addSetupInfo ("$ofedlogs/kmp_$package\_rpms_uninstall.log");
                    print_and_log_colored("See $ofedlogs/kmp_$package\_rpms_uninstall.log", 1, "RED");
                    exit $ERROR;
                }
            }
        }
    }

    if ( -d "/lib/modules/$kernel/kernel/drivers/net/mtnic" ) {
        print_and_log("Uninstalling mtnic driver...\n", (not $quiet));
        system("/sbin/rmmod mtnic > /dev/null 2>&1");
        system("/bin/rm -rf /lib/modules/$kernel/kernel/drivers/net/mtnic");
        system("/sbin/depmod $kernel > /dev/null 2>&1");
    }

}

sub install
{
    # Build and install selected RPMs
    for my $package ( @selected_packages ) {
        if ($packages_info{$package}{'internal'}) {
            my $parent = $packages_info{$package}{'parent'};
            if (not is_srpm_available($parent)) {
                print_and_log_colored("$parent source RPM is not available", 1, "RED");
                next;
            }
        }
        my $pname = $packages_info{$package}{'parent'};
        for my $ver (keys %{$main_packages{$pname}}) {
            if ($packages_info{$package}{'mode'} eq "user") {
                if (not $packages_info{$package}{'exception'}) {
                    if (not $packages_info{$package}{$ver}{'rpm_exist'}) {
                        build_rpm($package, $ver);
                    }

                    if (not $packages_info{$package}{$ver}{'rpm_exist'}) {
                        print_and_log_colored("$package $ver was not created", 1, "RED");
                        exit $ERROR;
                    }
                    if ($build_only) {
                        next;
                    }
                    if ((not $with_dkms_install) and $package =~ /-dkms/) {
                        print_and_log("Skipping install of DKMS user package $package $ver.\n", $verbose);
                        next;
                    }
                    print_and_log("Install $package $ver RPM:\n", $verbose);
                    install_rpm($package, $ver);
                }
            }
            else {
                # kernel modules
                if ($package =~ m/kernel-mft-mlnx/ and not $kmp) {
                    $package =~ s/-mlnx//;
                }
                if ($package eq "mlnx-en" and not $kmp) {
                    $package = "mlnx_en";
                }
                if (not $packages_info{$package}{$ver}{'rpm_exist'}) {
                    my $parent = $packages_info{$package}{'parent'};
                    print_and_log("Build $parent $ver RPM\n", $verbose);
                    build_kernel_rpm($parent, $ver);
                }
                if ($kmp and $package !~ /devel|debug|source|dkms/ and $package =~ m/ofa_kernel|knem|kernel-mft-mlnx|mlnx-en$/) {
                    my @kmp_packages;
                    if ($DISTRO =~ /SLES|SUSE/) {
                        @kmp_packages = <$RPMS/$package-kmp*.$rpm_distro.$arch.rpm>;
                    } else {
                        @kmp_packages = <$RPMS/kmod*$package-*.$rpm_distro.$arch.rpm>;
                    }
                    if (not @kmp_packages) {
                        my $parent = $packages_info{$package}{'parent'};
                        print_and_log("Build $parent $ver RPM\n", $verbose);
                        build_kernel_rpm($parent, $ver);
                    }
                }
                if (not $packages_info{$package}{$ver}{'rpm_exist'}) {
                    next if ($package =~ /devel|source/);
                    print_and_log_colored("$package $ver was not created", 1, "RED");
                    exit $ERROR;
                }
                if ($build_only) {
                    next;
                }
                if ((not $with_dkms_install) and $package =~ /-dkms/) {
                    print_and_log("Skipping install of DKMS package $package $ver.\n", $verbose);
                    next;
                }
                print_and_log("Install $package $ver RPM:\n", $verbose);
                install_kernel_rpm($package, $ver);
            }
        }
    }
    if ($speed_up_kmp) {
        if (-x "/sbin/weak-modules") {
            system("/sbin/depmod $kernel > /dev/null 2>&1");
            system("cat '$modules_list' | /sbin/weak-modules --add-modules");
            unlink $modules_list;
        }
    }
    for my $package ( @selected_packages ) {
        if ($packages_info{$package}{'do_not_release'}) {
            for my $rpm (@{$packages_info{$package}{'built_rpms'}}) {
                print_and_log("Removing ${rpm}...\n", 1);
                unlink $rpm if (-e $rpm);
            }
        }
    }
}

sub is_srpm_available
{
    my $name = shift;

    for my $ver (keys %{$main_packages{$name}}) {
        if ($main_packages{$name}{$ver}{'srpmpath'}) {
            return 1;
        }
    }

    return 0;
}

sub addSetupInfo
{
    my $log = shift @_;

    print_and_log("Collecting debug info...\n", (not $quiet));

    if (not open (LOG, ">> $log")) {
        print_and_log("-E- Can't open $log for appending!\n", 1);
        return;
    }

    print LOG "\n\n\n---------------- START OF DEBUG INFO -------------------\n";
    print LOG "Install command: $CMD\n";

    print LOG "\nVars dump:\n";
    print LOG "- ofedlogs: $ofedlogs\n";
    print LOG "- DISTRO: $DISTRO\n";
    print LOG "- dist_rpm: $dist_rpm\n";
    print LOG "- rpm_distro: $rpm_distro\n";
    print LOG "- arch: $arch\n";
    print LOG "- kernel: $kernel\n";
    print LOG "- kmp: $kmp\n";
    print LOG "- config: $config\n";
    print LOG "- target_cpu: $target_cpu\n";
    print LOG "- update: $update\n";

    print LOG "\nSetup info:\n";
    print LOG "\n- uname -r: " . `uname -r 2>&1`;
    print LOG "\n- uname -m: " . `uname -m 2>&1`;
    print LOG "\n- rpm -qf /etc/issue: " . `rpm -qf /etc/issue 2>&1`;
    print LOG "\n- cat /etc/issue: " . `cat /etc/issue 2>&1`;
    print LOG "\n- rpm -qf /etc/os-release: " . `rpm -qf /etc/os-release 2>&1`;
    print LOG "\n- cat /etc/os-release: " . `cat /etc/os-release 2>&1`;
    print LOG "\n- cat /proc/version: " . `cat /proc/version 2>&1`;
    print LOG "\n- gcc --version: " . `gcc --version 2>&1`;
    print LOG "\n- lspci -n | grep 15b3: " . `lspci -n 2>&1 | grep 15b3`;
    print LOG "\n- rpm -qa: " . `rpm -qa 2>&1`;

    print LOG "---------------- END OF DEBUG INFO -------------------\n";
    close (LOG);
}

sub ex
{
    my $cmd = shift @_;

    system("$cmd");
    my $res = $? >> 8;
    my $sig = $? & 127;
    if ($sig or $res) {
        print_and_log_colored("Command execution failed: $cmd", 1, "RED");
        exit 1;
    }
}


### MAIN AREA ###
sub main
{
    if ($print_available) {
        my @list = ();

        for my $srcrpm ( <$SRPMS*> ) {
            set_cfg ($srcrpm);
        }
        set_availability();

        if (!$install_option) {
            $install_option = 'all';
        }

        $config = $conf_dir . "/ofed-$install_option.conf";
        chomp $config;
        if ($install_option eq 'all') {
            @list = (@all_packages, @hidden_packages);
        }
        elsif ($install_option eq 'bluefield') {
            if ($kernel_only) {
                @list = (@bluefield_kernel_packages);
            } else {
                @list = (@bluefield_user_packages, @bluefield_kernel_packages);
            }
            @kernel_modules = (@bluefield_kernel_modules);
        }
        elsif ($install_option eq 'hpc') {
            @list = (@hpc_user_packages, @hpc_kernel_packages);
            @kernel_modules = (@hpc_kernel_modules);
        }
        elsif ($install_option eq 'hypervisor-os') {
            @list = (@hypervisor_user_packages, @hypervisor_kernel_packages);
            @kernel_modules = (@hypervisor_kernel_modules);
        }
        elsif ($install_option eq 'guest-os') {
            @list = (@guest_user_packages, @guest_kernel_packages);
            @kernel_modules = (@guest_kernel_modules);
        }
        elsif ($install_option =~ m/vma/) {
            if ($install_option eq 'vma') {
                @list = (@vma_user_packages, @vma_kernel_packages);
            } elsif ($install_option eq 'vmavpi') {
                @list = (@vmavpi_user_packages, @vma_kernel_packages);
            } elsif ($install_option eq 'vmaeth') {
                @list = (@vmaeth_user_packages, @vma_kernel_packages);
            }
            @kernel_modules = (@vma_kernel_modules);
        }
        elsif ($install_option eq 'xlio') {
            @list = (@xlio_user_packages, @xlio_kernel_packages);
            @kernel_modules = (@xlio_kernel_modules);
        }
        elsif ($install_option eq 'basic') {
            @list = (@basic_user_packages, @basic_kernel_packages);
            @kernel_modules = (@basic_kernel_modules);
        }
        elsif ($install_option eq 'kernel-only') {
            for my $package ( @all_packages ) {
                next if (($packages_info{$package}{'mode'} ne 'kernel') && ($packages_info{$package}{'name'} ne 'kernel-mft-dkms'));
                push (@list, $package);
            }
        }
        elsif ($install_option eq 'eth-only') {
            @list = (@eth_user_packages, @eth_kernel_packages);
            @kernel_modules = (@eth_kernel_modules);
        }
        elsif ($install_option =~ m/dpdk/) {
            @list = (@dpdk_user_packages, @dpdk_kernel_packages);
            @kernel_modules = (@dpdk_kernel_modules);
        }

        if ($with_bluefield and $install_option ne 'bluefield') {
            for my $package ( @bluefield_user_packages, @bluefield_kernel_packages ) {
                next if (grep /^$package$/, @list);
                my $parent = $packages_info{$package}{'parent'};
                next if ($kernel_only and $packages_info{$package}{'mode'} ne 'kernel');
                push (@list, $package);
            }
            for my $module ( @bluefield_kernel_modules ) {
                next if (/^$module$/, @kernel_modules);
                push (@kernel_modules, $module);
            }
        }

        if ($with_ovs) {
            for my $package (@openvswitch_packages) {
                next if (grep /^$package$/, @list);
                next if (not $packages_info{$package}{'available'});
                next if ($kernel_only and $packages_info{$package}{'mode'} ne 'kernel');
                push (@list, $package);
            }
        }

        @selected_by_user = (@list);
        @selected_modules_by_user = (@kernel_modules);
        add_enabled_pkgs_by_user();
        resolve_dependencies();
        open(CONFIG, ">$config") || die "Can't open $config: $!";;
        flock CONFIG, $LOCK_EXCLUSIVE;
        print_and_log("\nOFED packages: ", 1);
        for my $package ( @selected_packages ) {
            my $parent = $packages_info{$package}{'parent'};
            next if (not $packages_info{$package}{'available'} or not is_srpm_available($parent));
            print_and_log("$package available: $packages_info{$package}{'available'}\n", $verbose2);
            if ($package =~ /ofa_kernel/ and $package !~ /devel|source/) {
                print_and_log("\nKernel modules: ", 1);
                for my $module ( @selected_kernel_modules ) {
                    next if (not $kernel_modules_info{$module}{'available'});
                    print_and_log($module . ' ', 1);
                    print CONFIG "$module=y\n";
                }
                print_and_log("\nRPMs: ", 1);
            }
            print_and_log($package . ' ', 1);
            print CONFIG "$package=y\n";
        }
        flock CONFIG, $UNLOCK;
        close(CONFIG);
        print_and_log("\n", 1);
        print_and_log_colored("Created $config", 1, "GREEN");
        exit $SUCCESS;
    }

    warn("Logs dir: $ofedlogs\n");
    warn("General log file: $glog\n");
    my $num_selected = 0;

    if ($interactive) {
        my $inp;
        my $ok = 0;
        my $max_inp;

        while (! $ok) {
            $max_inp = show_menu("main");
            $inp = getch();

            if ($inp =~ m/[qQ]/ || $inp =~ m/[Xx]/ ) {
                die "Exiting\n";
            }
            if (ord($inp) == $KEY_ENTER) {
                next;
            }
            if ($inp =~ m/[0123456789abcdefABCDEF]/)
            {
                $inp = hex($inp);
            }
            if ($inp < 1 || $inp > $max_inp)
            {
                print "Invalid choice...Try again\n";
                next;
            }
            $ok = 1;
        }

        if ($inp == 1) {
            if (-e "$CWD/docs/${PACKAGE}_Installation_Guide.txt") {
                system("less $CWD/docs/${PACKAGE}_Installation_Guide.txt");
            }
            elsif (-e "$CWD/README.txt") {
                system("less $CWD/README.txt");
            }
            else {
                print RED "$CWD/docs/${PACKAGE}_Installation_Guide.txt does not exist...", RESET;
            }

            return 0;
        }
        elsif ($inp == 2) {
            for my $srcrpm ( <$SRPMS*> ) {
                set_cfg ($srcrpm);
            }

            # Set RPMs info for available source RPMs
            set_availability();
            $num_selected = select_packages();
            set_existing_rpms();
            resolve_dependencies();
            check_linux_dependencies();
            if (not $quiet) {
                print_selected();
            }
        }
        elsif ($inp == 3) {
            my $cnt = 0;
            for my $package ( @all_packages, @hidden_packages) {
                if (is_installed($package)) {
                    print "$package\n";
                    $cnt ++;
                }
            }
            if (not $cnt) {
                print "\nThere is no $PACKAGE software installed\n";
            }
            print GREEN "\nPress any key to continue...", RESET;
            getch();
            return 0;
        }
        elsif ($inp == 4) {
            ipoib_config();
            return 0;
        }
        elsif ($inp == 5) {
            uninstall();
            exit 0;
        }

    }
    else {
        for my $srcrpm ( <$SRPMS*> ) {
            next if ($srcrpm =~ /KMP/);
            set_cfg ($srcrpm);
        }

        for my $srcrpm ( <$SRPMS/KMP/*> ) {
            set_cfg ($srcrpm);
        }

        # Set RPMs info for available source RPMs
        set_availability();
        $num_selected = select_packages();
        set_existing_rpms();
        resolve_dependencies();
        check_linux_dependencies();
        if (not $quiet) {
            print_selected();
        }
    }

    if (not $num_selected) {
        print_and_log_colored("$num_selected packages selected. Exiting...", 1, "RED");
        exit 1;
    }
    print_and_log("Detected Linux Distribution: $DISTRO\n", $verbose3);

    print_and_log("This program will install the $PACKAGE package on your machine.\n"
        . "Note that all other Mellanox, OEM, OFED, RDMA or Distribution IB packages will be removed.\n"
        . "Those packages are removed due to conflicts with $PACKAGE, do not reinstall them.\n\n", (not $quiet)) if (not $update);

    # Uninstall the previous installations
    uninstall();
    my $vendor_ret;
    if (length($vendor_pre_install) > 0) {
	    print_and_log("\nRunning vendor pre install script: $vendor_pre_install\n", (not $quiet));
	    $vendor_ret = system ( "$vendor_pre_install", "CONFIG=$config",
		"RPMS=$RPMS", "SRPMS=$SRPMS", "PREFIX=$prefix", "TOPDIR=$TOPDIR", "QUIET=$quiet" );
	    if ($vendor_ret != 0) {
		    print_and_log_colored("\nExecution of vendor pre install script failed.", (not $quiet), "RED");
		    exit 1;
	    }
    }
    alios_remove_gcc_from_path();
    install();
    if ($build_only) {
        print_and_log "Build-only run finished successfully.\n", $verbose2;
        return;
    }

    system("/sbin/ldconfig > /dev/null 2>&1");

    if (-e "/etc/modprobe.conf.dist" and not $user_space_only) {
        open(MDIST, "/etc/modprobe.conf.dist") or die "Can't open /etc/modprobe.conf.dist: $!";
        my @mdist_lines;
        while (<MDIST>) {
            push @mdist_lines, $_;
        }
        close(MDIST);

        open(MDIST, ">/etc/modprobe.conf.dist") or die "Can't open /etc/modprobe.conf.dist: $!";
        foreach my $line (@mdist_lines) {
            chomp $line;
            if ($line =~ /^\s*install ib_core|^\s*alias ib|^\s*alias net-pf-26 ib_sdp/) {
                print MDIST "# $line\n";
            } else {
                print MDIST "$line\n";
            }
        }
        close(MDIST);
    }

    if (length($vendor_pre_uninstall) > 0) {
	    system "cp '$vendor_pre_uninstall' $prefix/sbin/vendor_pre_uninstall.sh";
    }
    if (length($vendor_post_uninstall) > 0) {
	    system "cp '$vendor_post_uninstall' $prefix/sbin/vendor_post_uninstall.sh";
    }
    if (length($vendor_post_install) > 0) {
	    print_and_log("\nRunning vendor post install script: $vendor_post_install\n", (not $quiet));
	    $vendor_ret = system ( "$vendor_post_install", "CONFIG=$config",
		"RPMS=$RPMS", "SRPMS=$SRPMS", "PREFIX=$prefix", "TOPDIR=$TOPDIR", "QUIET=$quiet");
	    if ($vendor_ret != 0) {
		    print_and_log_colored("\nExecution of vendor post install script failed.", (not $quiet), "RED");
		    exit 1;
	    }
    }

    if ($do_copy_udev) {
        copy_udev_rules($verbose2);
    }

    if ($kernel_modules_info{'ipoib'}{'selected'} and not $user_space_only) {
        ipoib_config();

        # Decrease send/receive queue sizes on 32-bit arcitecture
        # BUG: https://bugs.openfabrics.org/show_bug.cgi?id=1420
        if ($arch =~ /i[3-6]86/) {
            if (-e "/etc/modprobe.d/ib_ipoib.conf") {
                open(MODPROBE_CONF, ">>/etc/modprobe.d/ib_ipoib.conf");
                print MODPROBE_CONF "options ib_ipoib send_queue_size=64 recv_queue_size=128\n";
                close MODPROBE_CONF;
            }
        }

        # BUG: https://bugs.openfabrics.org/show_bug.cgi?id=1449
        if (-e "/etc/modprobe.d/ipv6") {
            open(IPV6, "/etc/modprobe.d/ipv6") or die "Can't open /etc/modprobe.d/ipv6: $!";
            my @ipv6_lines;
            while (<IPV6>) {
                push @ipv6_lines, $_;
            }
            close(IPV6);

            open(IPV6, ">/etc/modprobe.d/ipv6") or die "Can't open /etc/modprobe.d/ipv6: $!";
            foreach my $line (@ipv6_lines) {
                chomp $line;
                if ($line =~ /^\s*install ipv6/) {
                    print IPV6 "# $line\n";
                } else {
                    print IPV6 "$line\n";
                }
            }
            close(IPV6);
        }
    }

    if ( not $quiet ) {
        check_pcie_link();
    }

    if ($umad_dev_rw or $umad_dev_na) {
        set_umad_permissions($umad_dev_na);
    }

    my @openmpi_mca_params;
    # Openmpi post-install action
    if (-d "$prefix/mpi") {
        @openmpi_mca_params = `find $prefix/mpi -name openmpi-mca-params.conf 2> /dev/null`;
        for my $openmpi_conf (@openmpi_mca_params) {
            chomp $openmpi_conf;
            system("echo coll_fca_enable = 0 >> $openmpi_conf 2>&1");
            system("echo coll = ^ml >> $openmpi_conf");
        }
    }

    if (-e "/etc/infiniband/openib.conf") {
        my @lines;
        open(FD, "/etc/infiniband/openib.conf");
        while (<FD>) {
            push @lines, $_;
        }
        close (FD);
        open(FD, ">/etc/infiniband/openib.conf");
        foreach my $line (@lines) {
            chomp $line;
            if ($line =~ m/(^SDP_LOAD=|^QIB_LOAD=).*/) {
                    print FD "${1}no\n";
            } elsif ($line =~ m/(^SET_IPOIB_CM=).*/ and $with_vma) {
                # Set IPoIB Datagram mode in case of VMA installation
                print FD "SET_IPOIB_CM=no\n";
            } else {
                    print FD "$line\n";
            }
        }
        close (FD);
    }

    if ($DISTRO =~ m/OL/ and $kernel =~ m/2.6.32-279.19.1.el6/) {
        my @lines;
        open(FD, "/etc/infiniband/openib.conf");
        while (<FD>) {
            push @lines, $_;
        }
        close (FD);

        open(FD, ">/etc/infiniband/openib.conf");
        foreach my $line (@lines) {
            chomp $line;
            if ($line =~ m/(^RUN_SYSCTL=).*/) {
                    print FD "${1}no\n";
            } else {
                    print FD "$line\n";
            }
        }
        close (FD);
    }

    # Enable/disable mlnx_tune
    if ( -e "/etc/infiniband/openib.conf") {
        my @lines;
        open(FD, "/etc/infiniband/openib.conf");
        while (<FD>) {
            push @lines, $_;
        }
        close (FD);

        open(FD, ">/etc/infiniband/openib.conf");
        foreach my $line (@lines) {
            chomp $line;
            if ($line =~ m/(^RUN_MLNX_TUNE=).*/) {
                if ($enable_mlnx_tune) {
                    print FD "${1}yes\n";
                } else {
                    print FD "${1}no\n";
                }
            } else {
                    print FD "$line\n";
            }
        }
        close (FD);
    }

    my $mlnx_conf = "/etc/modprobe.d/mlnx.conf";
    if ($with_vma and -e "$mlnx_conf" and not $user_space_only) {
        my @lines;
        open(FD, "$mlnx_conf");
        while (<FD>) {
            push @lines, $_;
        }
        close (FD);
        open(FD, ">$mlnx_conf");
        foreach my $line (@lines) {
            chomp $line;
            print FD "$line\n" unless ($line =~ /disable_raw_qp_enforcement|fast_drop|log_num_mgm_entry_size/);
        }
        print FD "options mlx4_core fast_drop=1\n";
        print FD "options mlx4_core log_num_mgm_entry_size=-1\n";
        close (FD);
    }

    if ($enable_opensm) {
        system("systemctl enable opensmd  > /dev/null 2>&1");
    } else {
        system("systemctl disable opensmd > /dev/null 2>&1");
    }

    if (is_installed("ibacm")) {
        # Disable ibacm daemon by default
        system("chkconfig --del ibacm > /dev/null 2>&1");
    }

    if (grep /mlnx-ofa_kernel-modules/, @selected_packages) {
        if ($update_initrd_cmd) {
            print_and_log_colored("You may need to update your initramfs before next boot. To do that, run:\n", (not $quiet), "GREEN");
            print_and_log("   $update_initrd_cmd\n", (not $quiet));
        }
    }


    print_and_log_colored("\nInstallation finished successfully.", 1, "GREEN");
    if ($interactive) {
        print GREEN "\nPress any key to continue...", RESET;
        getch();
    }
    else {
        print "\n";
    }
}

while (1) {
    main();
    exit 0 if (not $interactive);
}
