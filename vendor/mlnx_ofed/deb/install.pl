#!/usr/bin/perl
#
# Copyright (c) 2013 Mellanox Technologies. All rights reserved.
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
use Cwd;
use Term::ANSIColor qw(:constants);

my $WDIR = dirname(Cwd::abs_path $0);
require("$WDIR/common.pl");

my $PREREQUISIT = "172";
my $MST_START_FAIL = "173";
my $NO_HARDWARE = "171";
my $SUCCESS = "0";
my $DEVICE_INI_MISSING = "2";
my $ERROR = "1";
my $EINVAL = "22";
my $ENOSPC = "28";
my $NONOFEDRPMS = "174";

$ENV{"LANG"} = "C";

$| = 1;

chdir $WDIR;
my $CWD     = getcwd;
my $DPKG = "/usr/bin/dpkg";
my $DPKG_QUERY = "/usr/bin/dpkg-query";
my $DPKG_BUILDPACKAGE = "/usr/bin/dpkg-buildpackage";
my $MODINFO = "/sbin/modinfo";
my $DPKG_FLAGS = "--force-confmiss";
my $DPKG_DEB = "/usr/bin/dpkg-deb";
my $BUILD_ENV = '';
my $enable_mlnx_tune = 0;
my $check_linux_deps = 1;

my $ifconf = "/etc/network/interfaces";
my $config_net_given = 0;
my $config_net = "";
my %ifcfg = ();
my $umad_dev_rw = 0;
my $umad_dev_na = 0;
my $config_given = 0;
my $conf_dir = $CWD;
my $config = $conf_dir . '/ofed.conf';
chomp $config;
my $install_option = 'all';
if (-e ".def_option" ) {
	$install_option = `cat .def_option 2>/dev/null`;
	chomp $install_option;
}
my $force_all = 0;
my $user_space_only = 0;
my $with_vma = 0;
my $with_xlio = 0;
my $print_available = 0;
my $force = 0;
my %disabled_packages;
my %force_enable_packages;
my %packages_deps = ();
my %modules_deps = ();
my $with_memtrack = 0;
my $with_dkms = 1;
my $with_gds = 1;
my $with_kmod_debug_symbols = 0;
my $force_dkms = 0;
my $build_only = 0;
my $uninstall = 1;
my $with_ovs = 0;
my $with_openvswitch = 0;
my $with_mlxdevm = 1;

my $kernel_elfutils_devel = 'libelf-dev';

# list of scripts to run for each package
my %package_pre_build_script = ();
my %package_post_build_script = ();
my %package_pre_install_script = ();
my %package_post_install_script = ();

$ENV{"DEBIAN_FRONTEND"} = "noninteractive";

my $CMD = "$0 " . join(' ', @ARGV);
my $enable_opensm = 0;
my @components = qw/kernel user/;

my $LOCK_EXCLUSIVE = 2;
my $UNLOCK         = 8;

my $PACKAGE     = 'OFED';
my $TMPDIR = "/tmp";

my $quiet = 0;
my $verbose = 1;
my $verbose2 = 0;
my $verbose3 = 0;
my %selected_for_uninstall;
my @dependant_packages_to_uninstall = ();
my %non_ofed_for_uninstall = ();

my $builddir = "/var/tmp";

my %main_packages = ();
my @selected_packages = ();
my @selected_modules_by_user = ();
my @selected_kernel_modules = ();
my $kernel_configure_options = '';
# list of the packages that will be installed (selected by user)
my @selected_by_user = ();
my @selected_to_install = ();

my $distro = "";
my $arch = `uname -m`;
chomp $arch;
my $kernel = `uname -r`;
chomp $kernel;
my $kernel_sources = "/lib/modules/$kernel/build";
chomp $kernel_sources;
my $kernel_given = 0;
my $kernel_source_given = 0;
my $cross_compiling = 0;
my $check_deps_only = 0;
my $print_distro = 0;
my $with_mlx5_ipsec = 1;
my $kernel_extra_args = "";
my $apt_extra_params = "-o Dpkg::Options::='--force-confdef' -o Dpkg::Options::='--force-confold'";
my $do_copy_udev = 0;
my $with_fwctl = 0;
my $with_virtiofs = 0;
my $with_dkms_install = 1;

if ($ENV{"ARCH"} ne "" and $ENV{"ARCH"} ne "$arch") {
	print "Detected cross compiling (local: $arch, target: $ENV{ARCH})\n\n";
	$arch = $ENV{"ARCH"};
	$arch =~ s/arm64/aarch64/g;
	$DPKG_BUILDPACKAGE = "$DPKG_BUILDPACKAGE -a$ENV{ARCH}";
	$cross_compiling = 1;
}

my $is_bf = `lspci -s 00:00.0 2> /dev/null | grep -wq "PCI bridge: Mellanox Technologies" && echo 1 || echo 0`;
chomp $is_bf;

my $with_bluefield = 0;
if  ($is_bf) {
	$with_bluefield = 1;
}

#
# parse options
#
while ( $#ARGV >= 0 ) {
	my $cmd_flag = shift(@ARGV);

	if ( $cmd_flag eq "--all" ) {
		$install_option = 'all';
		$force_all = 1;
	} elsif ( $cmd_flag eq "--bluefield" ) {
		# Do not override other install options to enable bluefield packages as an extension
		$install_option = 'bluefield' if (not $install_option or ($install_option eq 'all' and not $force_all));
		$with_bluefield = 1;
	} elsif ( $cmd_flag eq "--hpc" ) {
		$install_option = 'hpc';
	} elsif ( $cmd_flag eq "--basic" ) {
		$install_option = 'basic';
	} elsif ( $cmd_flag eq "--msm" ) {
		$install_option = 'msm';
		$enable_opensm = 1;
	} elsif ( $cmd_flag eq "--with-vma" and not ($install_option eq 'eth-only')) {
		$with_vma = 1;
	} elsif ( $cmd_flag eq "--vma" ) {
		$install_option = 'vma';
		$with_vma = 1;
	} elsif ( $cmd_flag eq "--vma-eth" ) {
		$install_option = 'vmaeth';
		$with_vma = 1;
	} elsif ( $cmd_flag eq "--vma-vpi" ) {
		$install_option = 'vmavpi';
		$with_vma = 1;
	} elsif ( $cmd_flag eq "--xlio" ) {
		$install_option = 'xlio';
		$with_xlio = 1;
	} elsif ( $cmd_flag eq "--with-xlio" ) {
		$with_xlio = 1;
	} elsif ( $cmd_flag eq "--guest" ) {
		$install_option = 'guest';
	} elsif ( $cmd_flag eq "--hypervisor" ) {
		$install_option = 'hypervisor';
	} elsif ( $cmd_flag eq "--kernel-only" ) {
		$install_option = 'kernel-only';
		@components = qw/kernel/;
	} elsif ( $cmd_flag eq "--user-space-only" ) {
		$user_space_only = 1;
		@components = qw/user/;
	} elsif ( $cmd_flag eq "--eth-only" ) {
		$install_option = 'eth-only';
	} elsif ( $cmd_flag eq "--dpdk" ) {
		$install_option = 'dpdk';
	} elsif ( $cmd_flag eq "--ovs" ) {
		$with_ovs = 1;
	} elsif ( $cmd_flag eq "--with-openvswitch" ) {
		$with_openvswitch = 1;
	} elsif ( $cmd_flag eq "--umad-dev-rw" ) {
		$umad_dev_rw = 1;
	} elsif ( $cmd_flag eq "--umad-dev-na" ) {
		$umad_dev_na = 1;
	} elsif ( $cmd_flag eq "--enable-opensm" ) {
		$enable_opensm = 1;
	} elsif ( $cmd_flag eq "-q" ) {
		$quiet = 1;
		$verbose = 0;
		$verbose2 = 0;
		$verbose3 = 0;
	} elsif ( $cmd_flag eq "-v" ) {
		$verbose = 1;
	} elsif ( $cmd_flag eq "-vv" ) {
		$verbose = 1;
		$verbose2 = 1;
	} elsif ( $cmd_flag eq "-vvv" ) {
		$verbose = 1;
		$verbose2 = 1;
		$verbose3 = 1;
	} elsif ( $cmd_flag eq "--force" ) {
		$force = 1;
	} elsif ( $cmd_flag eq "-n" or $cmd_flag eq "--net" ) {
		$config_net = shift(@ARGV);
		$config_net_given = 1;
	} elsif ( $cmd_flag eq "--with-memtrack" ) {
		$with_memtrack = 1;
	} elsif ($cmd_flag eq "--conf-dir") {
		$conf_dir = shift(@ARGV);
		mkpath([$conf_dir]) unless -d "$conf_dir";
		if (not $config_given) {
			$config = $conf_dir . '/ofed.conf';
		}
	} elsif ( $cmd_flag eq "-c" or $cmd_flag eq "--config" ) {
		$config = shift(@ARGV);
		$config_given = 1;
	} elsif ( $cmd_flag eq "-p" or $cmd_flag eq "--print-available" ) {
		$print_available = 1;
	} elsif ( $cmd_flag eq "--builddir" ) {
		$builddir = shift(@ARGV);
		$builddir = clean_path($builddir);
	} elsif ( $cmd_flag eq "--tmpdir" ) {
		$TMPDIR = shift(@ARGV);
		$TMPDIR = clean_path($TMPDIR);
	} elsif ( $cmd_flag eq "--enable-mlnx_tune" ) {
		$enable_mlnx_tune = 1;
	} elsif ( $cmd_flag eq "--without-mlx5-ipsec") {
                $with_mlx5_ipsec = 0;
        } elsif ( $cmd_flag eq "--without-depcheck" ) {
		$check_linux_deps = 0;
	} elsif ( $cmd_flag eq "--check-deps-only" ) {
		$check_deps_only = 1;
	} elsif ( $cmd_flag eq "--with-mlxdevm-mod" ) {
		$with_mlxdevm = 1;
	} elsif ( $cmd_flag eq "--without-mlxdevm-mod" ) {
		$with_mlxdevm = 0;
	} elsif ( $cmd_flag eq "--print-distro" ) {
		$print_distro = 1;
	} elsif ( $cmd_flag eq "--without-dkms" ) {
		$with_dkms = 0;
		$force_dkms = 0;
	} elsif ( $cmd_flag eq "--without-dkms-install" ) {
		$with_dkms_install = 0;
	} elsif ( $cmd_flag eq "--with-debug-symbols" ) {
		$with_kmod_debug_symbols = 1;
	} elsif ( $cmd_flag eq "--without-debug-symbols" ) {
		$with_kmod_debug_symbols = 0;
	} elsif ( $cmd_flag eq "--force-dkms" ) {
		$with_dkms = 1;
		$force_dkms = 1;
	} elsif ( $cmd_flag eq "--enable-gds" ) {
		# Left for backward compatibility
	} elsif ( $cmd_flag eq "-k" or $cmd_flag eq "--kernel" ) {
		$kernel = shift(@ARGV);
		$kernel_given = 1;
	} elsif ( $cmd_flag eq "-b" or $cmd_flag eq "--build-only" ) {
		$build_only = 1;
		$uninstall = 0;
	} elsif ( $cmd_flag eq "-s" or $cmd_flag eq "--kernel-sources" ) {
		$kernel_sources = shift(@ARGV);
		$kernel_source_given = 1;
	} elsif ( $cmd_flag =~ /--without|--disable/ ) {
		my $pckg = $cmd_flag;
		$pckg =~ s/--without-|--disable-//;
		$disabled_packages{$pckg} = 1;
	} elsif ( $cmd_flag =~ /--with-|--enable-/ ) {
		my $pckg = $cmd_flag;
		$pckg =~ s/--with-|--enable-//;
		$force_enable_packages{$pckg} = 1;
	} elsif ( $cmd_flag eq "--distro" ) {
		$distro = shift(@ARGV);
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
		$DPKG_FLAGS .= " $install_opt";
	} elsif ( $cmd_flag eq "--kernel-extra-args" ) {
		$kernel_extra_args = shift(@ARGV);
	} elsif ( $cmd_flag eq "--copy-ifnames-udev" ) {
		$do_copy_udev = 1;
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
		exit $EINVAL;
	}
}

if ($build_only and not (($install_option eq 'kernel-only') or ($install_option eq 'eth-only'))) {
    print RED "\nError: The '--build-only' option is supported only when '--kernel-only' option is given!", RESET "\n";
    print "To see list of supported options, run: $0 --help\n";
    exit 1;
}

if ($user_space_only and $install_option eq 'kernel-only') {
    print RED "\nError: The options '--kernel-only' and '--user-space-only' are incompatible. Aborting.", RESET "\n";
    exit 1;
}

if ((not $build_only) and (not $print_available) and (not $print_distro)) {
    check_root_user();
}

# packages to remove
my @remove_debs = qw(ar_mgr ar-mgr cc_mgr cc-mgr compat-dapl1 compat-dapl-dev dapl1 dapl1-utils dapl2-utils dapl-dev dump_pr dump-pr ibacm ibacm-dev ibsim ibsim-utils ibutils ibutils2 ibverbs-utils infiniband-diags infiniband-diags-compat infiniband-diags-guest libdapl2 libdapl-dev libibcm libibcm1 libibcm-dev libibdm1 libibdm-dev libibmad libibmad1 libibmad5 libibmad-dev libibmad-devel libibmad-static libibmad5-dbg libibnetdisc5 libibnetdisc-dev libibnetdisc5-dbg libibumad libibumad1 libibumad-dev libibumad-devel libibumad-static libibverbs1-dbg libipathverbs1 libipathverbs1-dbg libipathverbs-dev libmlx4 libmlx4-1 libmlx4-1-dbg libmlx4-dev libmlx5 libmlx5-1 libmlx5-1-dbg libmlx5-dev librxe-1 librxe-dev librxe-1-dbg libopensm libopensm2 libopensm2-dev libopensm-dev libopensm-devel librdmacm librdmacm1 librdmacm1-dbg librdmacm-dev libsdp1 libsdp-dev libumad2sim0 mlnx-ofed-kernel-dkms mlnx-ofed-kernel-modules mlnx-ofed-kernel-utils mlnx-tools ofed-docs ofed-scripts opensm opensm-libs opensm-doc perftest rdmacm-utils rds-tools sdpnetstat srptools mft kernel-mft-dkms mft-compat mft-devel mft-devmon mft-devmondb mft-int mft-oem mft-tests mstflint mxm ucx ucx-cuda fca openmpi openshmem mpitests knem knem-dkms xpmem xpmem-dkms ummunotify ummunotify-dkms libxlio libxlio-utils libxlio-dev libvma libvma-utils libvma-dev libvma-dbg dpcp sockperf srptools iser-dkms isert-dkms srp-dkms libmthca-dev libmthca1 libmthca1-dbg ibdump mlnx-ethtool mlnx-iproute2 mlnx-fw-updater knem-modules xpmem-modules iser-modules isert-modules srp-modules ummunotify-modules kernel-mft-modules libosmvendor libosmvendor4 libosmcomp libosmcomp3 mlnx-en mlnx-en-utils mlnx-en-dkms mlnx-en-modules mlnx-sdp-dkms mlnx-sdp-modules mlnx-rds-dkms mlnx-rds-modules mlnx-nfsrdma-dkms mlnx-nfsrdma-modules mlnx-nvme-dkms mlnx-nvme-modules fwctl-dkms fwctl-modules mlnx-rdma-rxe-dkms mlnx-rdma-rxe-modules libibumad3 libibumad3-dbg rdma-core libdisni-java-jni rshim-modules rshim-dkms rshim python3-pyverbs mlnx-tools nvme-snap spdk spdk-dev mlxbf-bootctl mlx-steering-dump ibarr libxpmem0 libxpmem-dev virtiofs-dkms virtiofs-modules);

my @immune_debs_list = map {"qemu-system-$_"} qw(arm misc mips ppc s390x sparc x86 x86-microvm x86-xen);
my %immune_debs = map { $_ => 1 } @immune_debs_list;

# required packages (will be always installed)
my @required_debs = qw(autotools-dev autoconf automake debhelper chrpath build-essential);

if ($kernel_given and not $kernel_source_given) {
    if (-d "/lib/modules/$kernel/build") {
        $kernel_sources = "/lib/modules/$kernel/build";
    }
    else {
        print RED "Provide path to the kernel sources for $kernel kernel.", RESET "\n";
        exit 1;
    }
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

sub warning($) {
	my $msg = shift @_;
	print_and_log_colored($msg, 1, "YELLOW");
}

print_and_log("Install command: $CMD\n", 0);

# disable DKMS if given kernel was not installed from deb package
if (not $force_dkms and $with_dkms and -d "$kernel_sources/scripts") {
	my $src_path = Cwd::abs_path "$kernel_sources/scripts";
	if (not $src_path) {
		$src_path = "$kernel_sources/scripts";
	}
	system("$DPKG -S '$src_path' >/dev/null 2>&1");
	my $res = $? >> 8;
	my $sig = $? & 127;
	if ($sig or $res) {
		print_and_log("DKMS is not supported for kernels which were not installed as DEB.\n", $verbose2);
		$with_dkms = 0;
	}
}

## set OS, arch
## don't auto-detect distro if it's provided by the user.
if ($distro eq "") {
        print_and_log("Distro was not provided, trying to auto-detect the current distro...\n", $verbose2);
        my $dist_os  = os_release('ID');
        if (not $dist_os) {
                print_and_log("/etc/os-release is missing or invalid\n");
        }
        my $dist_ver = os_release('VERSION_ID');
        $distro = "$dist_os$dist_ver";
        $distro = lc($distro);
        print_and_log("Auto-detected $distro distro.\n", $verbose2);
} else {
        $distro = lc($distro);
        print_and_log("Using provided distro: $distro\n", $verbose2);
}
if ($print_distro) {
    print_and_log("Printing distribution name and exiting:\n", $verbose2);
    print "$distro\n";
    exit 0;
}

if ($distro !~ m/debian | ubuntu | uos | velinux/x) {
        print_and_log_colored("Current operation system is not supported ($distro)!", 1, "RED");
        exit 1;
}

if ($distro =~ /uos/) {
	$with_dkms = 0;
}

#
# set kernel packages names
#
my $mlnX_ofed_kernel = "mlnx-ofed-kernel-dkms";
my $kernel_mft = "kernel-mft-dkms";
my $knem = "knem-dkms";
my $xpmem = "xpmem-dkms";
my $iser = "iser-dkms";
my $isert = "isert-dkms";
my $srp = "srp-dkms";
my $mlnx_en = "mlnx-en-dkms";
my $mlnx_nfsrdma = "mlnx-nfsrdma-dkms";
my $mlnx_nvme = "mlnx-nvme-dkms";
my $mlnx_rdma_rxe = "mlnx-rdma-rxe-dkms";
my $fwctl = "fwctl-dkms";
my $virtiofs = "virtiofs-dkms";

if (not $with_dkms) {
	$mlnX_ofed_kernel = "mlnx-ofed-kernel-modules";
	$kernel_mft = "kernel-mft-modules";
	$knem = "knem-modules";
	$xpmem = "xpmem-modules";
	$iser = "iser-modules";
	$isert = "isert-modules";
	$srp = "srp-modules";
	$mlnx_en = "mlnx-en-modules";
	$mlnx_nfsrdma = "mlnx-nfsrdma-modules";
	$mlnx_nvme = "mlnx-nvme-modules";
	$mlnx_rdma_rxe = "mlnx-rdma-rxe-modules";
	$fwctl = "fwctl-modules";
	$virtiofs = "virtiofs-modules";
}

my $kernel_escaped = $kernel;
$kernel_escaped =~ s/\+/\\\+/g;

my $libipsec_mb_dev  = "libipsec-mb-dev";
my $libipsec_mb = "libipsec-mb1";
if ($distro =~ /debian[89] | debian1[01] | ubuntu1 | ubuntu20/x) {
	$libipsec_mb = "libipsec-mb0";
} elsif ($distro =~ /ubuntu2[56]/) {
	$libipsec_mb = "libipsec-mb2";
}

if ($arch ne 'x86_64') {
	$libipsec_mb_dev  = "";
	$libipsec_mb = "";
}

my @mlnx_dpdk_packages = (
				"mlnx-dpdk",
				"mlnx-dpdk-dev",
			);

my @openvswitch_packages = (
				"openvswitch-common",
				"openvswitch-dev",
				"openvswitch-ipsec",
				"openvswitch-pki",
				"openvswitch-switch",
				"openvswitch-test",
				"openvswitch-vtep",
				"python3-openvswitch"
	);

push (@remove_debs, @mlnx_dpdk_packages);
if (($with_ovs or $with_openvswitch or $with_bluefield) and $arch =~ /x86_64|aarch64/) {
	push (@remove_debs, @openvswitch_packages);
}

my @ibverbs_packages = (
				"libibverbs",
				"libibverbs1",
				"libibverbs-dev",
				"ibverbs-providers"
	);

if ($distro !~ /ubuntu2[4-9]|debian1[3-9]/) {
	push (@remove_debs, @ibverbs_packages);
}

# custom packages
my @all_packages = (
				"ofed-scripts",
				"mlnx-tools",
				"mlnx-ofed-kernel-utils", "$mlnX_ofed_kernel",
				"$iser",
				"$isert",
				"$srp",
				"$mlnx_nfsrdma",
				"$mlnx_nvme",
				"$mlnx_rdma_rxe",
				"$fwctl",
				"$virtiofs",
				"libibverbs1", "ibverbs-utils", "libibverbs-dev",
				"ibverbs-providers",
				"libibumad3", "libibumad-dev",
				"ibacm",
				"librdmacm1", "rdmacm-utils", "librdmacm-dev",
				"mstflint",
				"libibmad5", "libibmad-dev",
				"opensm", "libopensm", "opensm-doc", "libopensm-devel",
				"infiniband-diags",
				"mft", "$kernel_mft",
				"ibacm",
				"perftest",
				"ibutils2",
				"cc-mgr",
				"ar-mgr",
				"dump-pr",
				"ibsim", "ibsim-doc",
				"ucx",
				"ucx-cuda",
				"openmpi",
				"knem", "$knem",
				"xpmem", "$xpmem",
				"libxpmem0", "libxpmem-dev",
				"libdapl2", "dapl2-utils", "libdapl-dev",
				"libvma", "libvma-utils", "libvma-dev",
				"libxlio", "libxlio-utils", "libxlio-dev",
				"dpcp",
				"sockperf",
				"srptools",
				"mlnx-ethtool",
				"mlnx-iproute2",
				"rshim",
				"ibarr",
);

my @basic_packages = (
				"ofed-scripts",
				"mlnx-tools",
				"mlnx-ofed-kernel-utils", "$mlnX_ofed_kernel",
				"$iser",
				"$isert",
				"$srp",
				"$mlnx_nfsrdma",
				"$mlnx_nvme",
				"$mlnx_rdma_rxe",
				"$fwctl",
				"$virtiofs",
				"libibverbs1", "ibverbs-utils", "libibverbs-dev",
				"ibverbs-providers",
				"libibumad3", "libibumad-dev",
				"ibacm",
				"librdmacm1", "rdmacm-utils", "librdmacm-dev",
				"mstflint",
				"libibmad5", "libibmad-dev",
				"opensm", "libopensm", "opensm-doc", "libopensm-devel",
				"infiniband-diags",
				"mft", "$kernel_mft",
				"srptools",
				"rshim",
				"mlnx-ethtool",
				"mlnx-iproute2",
);

my @bluefield_packages = (
				"ofed-scripts",
				"mlnx-tools",
				"mlnx-ofed-kernel-utils", "$mlnX_ofed_kernel",
				"$iser",
				"$isert",
				"$srp",
				"$mlnx_nvme",
				"$mlnx_rdma_rxe",
				"$fwctl",
				"$virtiofs",
				"libibverbs1", "ibverbs-utils", "libibverbs-dev",
				"ibverbs-providers",
				"libibumad3", "libibumad-dev",
				"ibacm",
				"perftest",
				"ucx",
				"ucx-cuda",
				"knem", "$knem",
				"xpmem", "$xpmem",
				"libxpmem0", "libxpmem-dev",
				"ibutils2",
				"librdmacm1", "rdmacm-utils", "librdmacm-dev",
				"mstflint",
				"libibmad5", "libibmad-dev",
				"opensm", "libopensm", "opensm-doc", "libopensm-devel",
				"infiniband-diags",
				"mft", "$kernel_mft",
				"srptools",
				"mlnx-ethtool",
				"mlnx-iproute2",
				"nvme-snap",
				"spdk",
				"spdk-dev",
				"mlxbf-bootctl",
				"rshim",
);

my @hpc_packages = (
				@basic_packages,
				"ibacm",
				"perftest",
				"ibutils2",
				"cc-mgr",
				"ar-mgr",
				"dump-pr",
				"ibsim", "ibsim-doc",
				"ucx",
				"ucx-cuda",
				"openmpi",
				"knem", "$knem",
				"xpmem", "$xpmem",
				"libxpmem0", "libxpmem-dev",
				"libdapl2", "dapl2-utils", "libdapl-dev",
);

my @xlio_packages = (
				@basic_packages,
				"perftest",
				"ibutils2",
				"cc-mgr",
				"ar-mgr",
				"dump-pr",
				"ibsim", "ibsim-doc",
				"libxlio", "libxlio-utils", "libxlio-dev",
				"dpcp",
				"sockperf",
				"rshim",
);

my @vma_packages = (
				@basic_packages,
				"perftest",
				"ibutils2",
				"cc-mgr",
				"ar-mgr",
				"dump-pr",
				"ibsim", "ibsim-doc",
				"libvma", "libvma-utils", "libvma-dev",
				"dpcp",
				"sockperf",
				"rshim",
);

my @vmavpi_packages = (
				@basic_packages,
				"perftest",
				"ibutils2",
				"cc-mgr",
				"ar-mgr",
				"dump-pr",
				"ibsim", "ibsim-doc",
				"libvma", "libvma-utils", "libvma-dev",
				"dpcp",
				"sockperf",
				"rshim",
);

my @vmaeth_packages = (
				"ofed-scripts",
				"mlnx-tools",
				"mlnx-ofed-kernel-utils", "$mlnX_ofed_kernel",
				"$iser",
				"$isert",
				"$srp",
				"$mlnx_nfsrdma",
				"$mlnx_nvme",
				"$mlnx_rdma_rxe",
				"$fwctl",
				"$virtiofs",
				"libibverbs1", "ibverbs-utils", "libibverbs-dev",
				"ibverbs-providers",
				"libibumad3", "libibumad-dev",
				"ibacm",
				"librdmacm1", "rdmacm-utils", "librdmacm-dev",
				"mstflint",
				"mft", "$kernel_mft",
				"libvma", "libvma-utils", "libvma-dev",
				"dpcp",
				"sockperf",
				"rshim",
				"mlnx-ethtool",
				"mlnx-iproute2",
);

my @guest_packages = (
				@basic_packages,
				"ibacm",
				"perftest",
				"libdapl2", "dapl2-utils", "libdapl-dev",
				"ucx",
				"ucx-cuda",
				"openmpi",
				"knem", "$knem",
				"xpmem", "$xpmem",
				"libxpmem0", "libxpmem-dev",
);

my @hypervisor_packages = (
				@basic_packages,
				"ibacm",
				"perftest",
				"libdapl2", "dapl2-utils", "libdapl-dev",
				"ibutils2",
				"ibarr",
);

my @eth_packages = (
				"$mlnx_en",
				"mlnx-tools",
				"mlnx-en-utils",
				"mstflint",
);

my @dpdk_packages = (
				"ofed-scripts", "mstflint", "mlnx-tools",
				"mlnx-ofed-kernel-utils", "$mlnX_ofed_kernel",
				"libibverbs1", "ibverbs-utils", "libibverbs-dev",
				"ibverbs-providers",
				"librdmacm1", "rdmacm-utils", "librdmacm-dev", "ibacm",
);

##
my %kernel_packages = ("$mlnX_ofed_kernel" => {'ko' => ["mlx5_ib", "mlx5_core"]},
			"$mlnx_en" => {'ko' => ["mlx5_core"]},
			"$knem" => {'ko' => ["knem"]},
			"$xpmem" => {'ko' => ["xpmem"]},
			"$kernel_mft" => {'ko' => ["mst_pci", "mst_pciconf"]},
			"$iser" => {'ko' => ["ib_iser"]},
			"$isert" => {'ko' => ["ib_isert"]},
			"$srp" => {'ko' => ["ib_srp"]},
			"$mlnx_nfsrdma" => {'ko' => ["rpcrdma"]},
			"$mlnx_nvme" => {'ko' => ["nvme_rdma"]},
			"$mlnx_rdma_rxe" => {'ko' => ["rdma_rxe"]},
			"$fwctl" => {'ko' => ["fwctl", "mlx5_fwctl"]},
			"$virtiofs" => {'ko' => ["virtiofs"]},
			);

my $DEBS  = "$CWD/DEBS/$distro/$arch";
chomp $DEBS;
mkpath(["$DEBS"]);

my $cuda_version = "13-2";
if ($distro =~ /ubuntu20/) {
	$cuda_version = "12-8";
} elsif ($distro =~ /ubuntu26/) {
        $cuda_version = "13-3";
}

my $cuda_dir_name = $cuda_version;
$cuda_dir_name =~ y/-/./;
my $cuda_top_dir = "";

# OS specific package names
my $module_tools = "kmod";
my $libssl = "libssl3";
my $libssl_devel = "libssl-dev";
my @rdmacore_python = qw/cython3 python3-dev/;
my @libsystemd_dev = qw/libsystemd-dev/;
my $systemd_dev = "";
my $libelf = "libelf1";
my $libfuse2 = "libfuse2";
my $libpcap = "libpcap0.8";
if ($distro =~ /ubuntu2[4-6] | debian13/x) {
	$systemd_dev = "systemd-dev";
}
if ($distro =~ /ubuntu1[0-7] | debian[5-9] | debian10/x) {
	@rdmacore_python = qw/python/; # older systems have no pyverbs
} else {
	push @dpdk_packages, "python3-pyverbs";
}
if ($distro =~ /ubuntu1[0-4] | debian[5-6]/x) {
	# Technically: libnih, practically: optional dependency:
	@libsystemd_dev = ();
}
if ( $distro =~ /debian6|ubuntu1[0-2]/) {
	$module_tools = "module-init-tools";
}
if ($distro =~ /debian9/) {
	$libssl = "libssl1.0.2";
	$libssl_devel = "libssl1.0-dev";
} elsif ($distro =~ /ubuntu16/) {
	$libssl = "libssl1.0.0";
} elsif ($distro =~ /ubuntu(19|2[01]) | debian1[0-1] | uos/x) {
	$libssl = "libssl1.1";
} elsif ($distro =~ /ubuntu18/) {
	$libssl = "libssl1.0.0";
} elsif ($distro =~ /ubuntu2[45678] | debian1[345]/x) {
	$libssl = "libssl3t64";
	$libelf = "libelf1t64";
	$libfuse2 = "libfuse2t64";
	$libpcap = "libpcap0.8t64";
}

my $linux_headers = "linux-headers-generic";
if ($distro =~ /debian/) {
	my $debian_arch=`dpkg-architecture -qDEB_TARGET_ARCH`;
	chomp $debian_arch;
	$linux_headers = "linux-headers-$debian_arch";
}

my $libudev = "libudev1";
my $libudev_devel = "libudev-dev";

my $dh_systemd = "debhelper";
if ($distro =~ /ubuntu1[0-7] | debian[4-8]/x) {
	$dh_systemd = "dh-systemd";
}

my $libgfortran = "libgfortran5";
if ($distro =~ /ubuntu18/x) {
	$libgfortran = "libgfortran4";
} elsif ($distro =~ /ubuntu1[46] | debian[89]/x) {
	$libgfortran = "libgfortran3";
}

if ($with_ovs and ($arch !~ /x86_64|aarch64/ or $distro !~ /debian1[0-2] | ubuntu2[0234].04/x)) {
	print YELLOW "\nWARNING: The '--ovs' option is supported only on Ubuntu 20.04 and Debian10 and above, x86_64 and aarch64. Disabling...", RESET "\n";
	$with_ovs = 0;
}

if ($with_gds and $distro !~ /ubuntu(18\.04|2[0246]\.04)|debian/) {
	$with_gds = 0;
}

my $libpython = "";
if ($distro =~ /debian10|uos/) {
	$libpython = "libpython3.7";
} elsif ($distro =~ /debian11/) {
	$libpython = "libpython3.9";
} elsif ($distro =~ /ubuntu24/x) {
	$libpython = "libpython3.12t64";
}

my $dh_dkms = "dh-dkms";
if ($distro =~ /ubuntu22|debian1[01]/) {
	$dh_dkms = "";
}

my $valgrind = "valgrind";
if ($arch =~ /riscv/) {
	$valgrind = "";
}

###############

# define kernel modules
my @basic_kernel_modules = ("core", "mlxfw", "mlx5", "ipoib", "mlxdevm");
my @ulp_modules = ("sdp", "srp", "srpt", "rds", "iser", "e_ipoib", "nfsrdma", 'isert', "mlxdevm");
my @kernel_modules = (@basic_kernel_modules, @ulp_modules);
my @bluefield_kernel_modules = ("core", "mlxfw", "mlx5", "ipoib", "mlx5_fpga_tools", "srp", "iser", "isert", "mlxdevm");
my @hpc_kernel_modules = (@basic_kernel_modules);
my @vma_kernel_modules = (@basic_kernel_modules);
my @xlio_kernel_modules = (@basic_kernel_modules);
my @dpdk_kernel_modules = (@basic_kernel_modules);
my @hypervisor_kernel_modules = ("core","mlxfw","mlx5","ipoib","srp","iser", 'isert', "mlxdevm");
my @guest_kernel_modules = ("core","mlxfw","mlx5","ipoib","srp","iser", 'isert', "mlxdevm");
my @eth_kernel_modules = ("core", "mlxfw", "mlx5", "mlxdevm");

# which modules are required for the standalone module rpms
my %standalone_kernel_modules_info = (
		"$iser" => ["core", "ipoib"],
		"$isert" => ["core", "ipoib"],
		"$srp" => ["core", "ipoib"],
		"$mlnx_nfsrdma" => ["core"],
		"$mlnx_nvme" => ["core"],
		"$mlnx_rdma_rxe" => ["core"],
		"$fwctl" => ["core"],
		"$virtiofs" => ["core"],
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
			'mlx5_fpga_tools' =>
			{ name => "mlx5_fpga_tools", available => 1, selected => 0,
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
			'mlxdevm' =>
			{ name => "mlxdevm", available => 1, selected => 0,
			included_in_rpm => 0, requires => ["mlx5"], },
);

# define packages
my %packages_info = (
			'ar-mgr' =>
				{
				dist_req_build => ["libstdc++6"],
				dist_req_inst => ["libstdc++6"],
				ofa_req_build => ["libopensm", "libopensm-devel", "ibutils2"],
				ofa_req_inst => ["opensm", "ibutils2"],
				},
			'cc-mgr' =>
				{
				dist_req_build => ["libstdc++6"],
				dist_req_inst => ["libstdc++6"],
				ofa_req_build => ["libopensm", "libopensm-devel", "ibutils2"],
				ofa_req_inst => ["opensm", "ibutils2"],
				},
			'dump-pr' =>
				{
				dist_req_build => ["libstdc++6"],
				dist_req_inst => ["libstdc++6"],
				ofa_req_build => ["libopensm", "libopensm-devel"],
				ofa_req_inst => ["opensm"],
				},
			'ibacm' =>
				{
				parent => "rdma-core",
				ofa_req_build => ["libibverbs-dev", "libibumad-dev", "$mlnX_ofed_kernel"],
				ofa_req_inst => ["libibverbs1", "libibumad3", "rdma-core"],
				},
			'ibsim-doc' =>
				{
				parent => "ibsim",
				ofa_req_build => ["libibmad-dev", "libibumad-dev"],
				ofa_req_inst => ["libibmad5", "libibumad3"],
				},
			'ibsim' =>
				{
				parent => "ibsim",
				ofa_req_build => ["libibmad-dev", "libibumad-dev"],
				ofa_req_inst => ["libibmad5", "libibumad3"],
				},
			'ibutils2' =>
				{
				dist_req_build => ["tcl-dev", "tk-dev", "libstdc++6"],
				dist_req_inst => ["tcl", "tk", "libstdc++6"],
				ofa_req_build => ["libibumad-dev"],
				ofa_req_inst => ["libibumad3"],
				},
			'infiniband-diags' =>
				{
				parent => "rdma-core",
				dist_req_build => ["libglib2.0-dev"],
				dist_req_inst => ["libglib2.0-0"],
				ofa_req_build => ["libibumad-dev", "libibmad-dev"],
				ofa_req_inst => ["libibumad3", "libibnetdisc5", "libibmad5"],
				},
			'kernel-mft-dkms' =>
				{
				parent => "kernel-mft",
				mode => "kernel",
				dist_req_build => ["dkms", "gcc", "make"],
				dist_req_inst => ["dkms", "gcc", "make"],
				},
			'kernel-mft' =>
				{
				parent => "kernel-mft",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				},
			'kernel-mft-modules' =>
				{
				parent => "kernel-mft",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				},
			'knem-dkms' =>
				{
				parent => "knem",
				mode => "kernel",
				dist_req_build => ["debhelper", "pkg-config", "bzip2", "dh-autoreconf", "dkms"],
				dist_req_inst => ["dkms", "gcc", "make", "pkg-config", "libc6-dev"],
				},
			'knem-modules' =>
				{
				parent => "knem",
				mode => "kernel",
				dist_req_build => ["debhelper", "pkg-config", "bzip2", "dh-autoreconf"],
				},
			'knem' =>
				{
				parent => "knem",
				mode => "kernel",
				dist_req_build => ["debhelper", "pkg-config", "bzip2", "dh-autoreconf"],
				dist_req_inst => ["pkg-config"],
				ofa_req_inst => [$knem],
				},
			'xpmem-dkms' =>
				{
				parent => "xpmem",
				mode => "kernel",
				dist_req_build => ["gcc", "make", "$dh_dkms"],
				dist_req_inst => ["dkms", "gcc", "make", "libc6-dev"],
				},
			'xpmem-modules' =>
				{
				parent => "xpmem",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				},
			'xpmem' =>
				{
				parent => "xpmem",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				ofa_req_inst => [$xpmem],
				},
			'libxpmem0' =>
				{
				parent => "xpmem-lib",
				dist_req_build => ["dh-autoreconf"],
				},
			'libxpmem-dev' =>
				{
				parent => "xpmem-lib",
				dist_req_build => ["dh-autoreconf"],
				ofa_req_inst => ["libxpmem0"],
				},
			'dapl' =>
				{
				parent => "dapl",
				available => 0,
				ofa_req_build => ["librdmacm-dev", "libibverbs-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1"],
				},
			'dapl2-utils' =>
				{
				parent => "dapl",
				ofa_req_build => ["librdmacm-dev", "libibverbs-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1"],
				},
			'libdapl-dev' =>
				{
				parent => "dapl",
				ofa_req_build => ["librdmacm-dev", "libibverbs-dev"],
				ofa_req_inst => ["libdapl2", "librdmacm1"],
				},
			'libdapl2' =>
				{
				parent => "dapl",
				ofa_req_build => ["librdmacm-dev", "libibverbs-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1"],
				},
			'libibmad-dev' =>
				{
				parent => "rdma-core",
				ofa_req_build => ["libibumad-dev"],
				ofa_req_inst => ["libibmad5"],
				},
			'libibmad5' =>
				{
				parent => "rdma-core",
				dist_req_build => ["libtool"],
				ofa_req_build => ["libibumad-dev"],
				ofa_req_inst => ["libibumad3"],
				},
			'libibnetdisc5' =>
				{
				parent => "rdma-core",
				ofa_req_inst => ["libibmad5", "libibumad3"],
				},
			'libibnetdisc-dev' =>
				{
				parent => "rdma-core",
				ofa_req_inst => ["libibnetdisc5"],
				},
			'ibverbs-utils' =>
				{
				parent => "rdma-core",
				ofa_req_inst => ["libibverbs1"],
				},
			'libibverbs-dev' =>
				{
				parent => "rdma-core",
				ofa_req_inst => ["libibverbs1", "ibverbs-providers"],
				},
			'libibverbs1' =>
				{
				parent => "rdma-core",
				dist_req_build => ["libnl-3-dev", "libnl-route-3-dev", "pkg-config"],
				dist_req_inst => ["libnl-3-200", "libnl-route-3-200", "adduser"],
				ofa_req_inst => ["rdma-core"],
				},

			'librdmacm-dev' =>
				{
				parent => "rdma-core",
				ofa_req_build => ["libibverbs-dev"],
				ofa_req_inst => ["librdmacm1", "rdma-core", "libibverbs-dev"],
				},
			'librdmacm1' =>
				{
				parent => "rdma-core",
				ofa_req_build => ["libibverbs-dev"],
				ofa_req_inst => ["libibverbs1", "rdma-core"],
				},
			'dpcp' =>
				{
				dist_req_build => ["pkg-config", "automake", "autoconf", "libtool"],
				ofa_req_build => ["libibverbs-dev"],
				ofa_req_inst => ["libibverbs1", "ibverbs-providers"],
				},
			'libvma' =>
				{
				parent => "libvma",
				dist_req_build => ["libnl-3-dev"],
				dist_req_inst => ["libnl-3-200"],
				ofa_req_build => ["librdmacm1", "librdmacm-dev", "libibverbs1", "libibverbs-dev"],
				ofa_req_inst => ["librdmacm1", "libibverbs1", "ibverbs-providers"],
				},
			'libvma-utils' =>
				{
				parent => "libvma",
				ofa_req_build => ["librdmacm1", "librdmacm-dev", "libibverbs1", "libibverbs-dev"],
				ofa_req_inst => ["librdmacm1", "libibverbs1", "libvma"],
				},
			'libvma-dev' =>
				{
				parent => "libvma",
				ofa_req_build => ["librdmacm1", "librdmacm-dev", "libibverbs1", "libibverbs-dev"],
				ofa_req_inst => ["librdmacm1", "libibverbs1", "libvma"],
				},
			'libxlio' =>
				{
				parent => "libxlio",
				dist_req_build => ["libnl-3-dev"],
				dist_req_inst => ["libnl-3-200"],
				ofa_req_build => ["librdmacm1", "librdmacm-dev", "libibverbs1", "libibverbs-dev", "dpcp"],
				ofa_req_inst => ["librdmacm1", "libibverbs1", "ibverbs-providers", "dpcp"],
				},
			'libxlio-utils' =>
				{
				parent => "libxlio",
				ofa_req_build => ["librdmacm1", "librdmacm-dev", "libibverbs1", "libibverbs-dev", "dpcp"],
				ofa_req_inst => ["librdmacm1", "libibverbs1", "libxlio"],
				},
			'libxlio-dev' =>
				{
				parent => "libxlio",
				ofa_req_build => ["librdmacm1", "librdmacm-dev", "libibverbs1", "libibverbs-dev", "dpcp"],
				ofa_req_inst => ["librdmacm1", "libibverbs1", "libxlio"],
				},

			'sockperf' =>
				{
				dist_req_build => ["doxygen"],
				},

			'rshim' =>
				{
				dist_req_build => ["autoconf", "autotools-dev", "pkg-config", "devscripts", "fakeroot", "libpci-dev", "libusb-1.0-0-dev", "libfuse-dev", $systemd_dev],
				dist_req_inst => ["libpci3", "libusb-1.0-0", "$libfuse2"],
				},

			'mft' =>
				{
				},
			'mlnx-ofed-kernel' =>
				{
				parent => "mlnx-ofed-kernel",
				available => 0,
				mode => "kernel",
				dist_req_build => ["dkms", "quilt", "make", "gcc"],
				dist_req_inst => ["dkms", "quilt", "make", "gcc", "coreutils", "pciutils", "grep", "perl", "procps", "$module_tools", "lsof"],
				ofa_req_build => ["$mlnX_ofed_kernel", "mlnx-ofed-kernel-utils"],
				ofa_req_inst => ["$mlnX_ofed_kernel", "mlnx-ofed-kernel-utils"],
				soft_req => ["ofed-scripts"],
				},
			'mlnx-ofed-kernel-dkms' =>
				{
				parent => "mlnx-ofed-kernel",
				mode => "kernel",
				dist_req_build => ["dkms", "quilt", "make", "gcc"],
				dist_req_inst => ["dkms", "quilt", "make", "gcc", "coreutils", "pciutils", "grep", "perl", "procps", "$module_tools", "lsof", "libc6-dev"],
				ofa_req_inst => ["ofed-scripts", "mlnx-ofed-kernel-utils"],
				soft_req => ["ofed-scripts"],
				},
			'mlnx-ofed-kernel-modules' =>
				{
				parent => "mlnx-ofed-kernel",
				mode => "kernel",
				dist_req_build => ["quilt", "make", "gcc"],
				dist_req_inst => ["quilt", "make", "gcc", "coreutils", "pciutils", "grep", "perl", "procps", "$module_tools", "lsof"],
				ofa_req_inst => ["ofed-scripts", "mlnx-ofed-kernel-utils"],
				soft_req => ["ofed-scripts"],
				},
			'mlnx-ofed-kernel-utils' =>
				{
				parent => "mlnx-ofed-kernel",
				mode => "kernel",
				dist_req_build => ["quilt", "make", "gcc", "$dh_dkms"],
				dist_req_inst => ["ethtool", "coreutils", "pciutils", "grep", "perl", "procps", "$module_tools", "lsof"],
				ofa_req_inst => ["ofed-scripts", "mlnx-tools"],
				soft_req => ["ofed-scripts"],
				},

			# eth only
			'mlnx-en' =>
				{
				parent => "mlnx-en",
				available => 0,
				mode => "kernel",
				dist_req_build => ["dkms", "quilt", "make", "gcc"],
				dist_req_inst => ["dkms", "quilt", "make", "gcc", "coreutils", "pciutils", "grep", "perl", "procps", "$module_tools", "lsof"],
				ofa_req_build => ["$mlnx_en", "mlnx-en-utils"],
				ofa_req_inst => ["$mlnx_en", "mlnx-en-utils"],
				soft_req => ["ofed-scripts"],
				},
			'mlnx-en-dkms' =>
				{
				parent => "mlnx-en",
				mode => "kernel",
				dist_req_build => ["dkms", "quilt", "make", "gcc"],
				dist_req_inst => ["dkms", "quilt", "make", "gcc", "coreutils", "pciutils", "grep", "perl", "procps", "$module_tools", "lsof", "libc6-dev"],
				ofa_req_inst => ["mlnx-en-utils"],
				soft_req => ["ofed-scripts"],
				},
			'mlnx-en-modules' =>
				{
				parent => "mlnx-en",
				mode => "kernel",
				dist_req_build => ["quilt", "make", "gcc"],
				dist_req_inst => ["quilt", "make", "gcc", "coreutils", "pciutils", "grep", "perl", "procps", "$module_tools", "lsof"],
				ofa_req_inst => ["mlnx-en-utils"],
				soft_req => ["ofed-scripts"],
				},
			'mlnx-en-utils' =>
				{
				parent => "mlnx-en",
				mode => "kernel",
				dist_req_build => ["quilt", "make", "gcc"],
				dist_req_inst => ["quilt", "make", "gcc", "coreutils", "pciutils", "grep", "perl", "procps", "$module_tools", "lsof"],
				ofa_req_inst => ['ofed-scripts', "mlnx-tools"],
				soft_req => ["ofed-scripts"],
				},
			'mlnx-tools' =>
				{
				dist_req_build => ["python3", "dh-python"],
				dist_req_inst => ["python3"],
				},

			'mstflint' =>
				{
				dist_req_build => ["$libssl_devel"],
				dist_req_inst => ["$libssl"],
				},
			'ucx' =>
				{
				parent => "ucx",
				dist_req_build => ["libstdc++6", "pkg-config", "libnuma-dev"],
				dist_req_inst => ["libstdc++6", "pkg-config"],
				ofa_req_build => ["libibverbs-dev", "librdmacm-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1"],
				},
			'ucx-cuda' =>
				{
				parent => "ucx",
				available => 0,
				dist_req_build => ["libstdc++6", "pkg-config", "libnuma-dev",
					"cuda-libraries-dev-$cuda_version"],
				dist_req_inst => ["cuda-cudart-$cuda_version"],
				ofa_req_build => ["libibverbs-dev", "librdmacm-dev"],
				ofa_req_inst => ["ucx"],
				},

			'ofed-scripts' =>
				{
				},
			'openmpi' =>
				{
				dist_req_build => ["libstdc++6", "$libgfortran", "libevent-dev", "libhwloc-dev"],
				dist_req_inst => ["libstdc++6", "$libgfortran", "gfortran"],
				ofa_req_build => ["libibverbs-dev", "librdmacm-dev","ucx", "knem", "libibmad-dev"],
				ofa_req_inst => ["libibverbs1", "ucx", "knem", "libibmad5"],
				soft_req => ["knem"],
				},
			'opensm' =>
				{
				parent => "opensm",
				dist_req_build => ["bison", "flex"],
				dist_req_inst => ["bison", "flex"],
				ofa_req_build => ["libopensm", "libibumad-dev"],
				ofa_req_inst => ["libopensm", "libibumad3"],
				},
			'opensm-doc' =>
				{
				parent => "opensm",
				dist_req_build => ["bison", "flex"],
				dist_req_inst => ["bison", "flex"],
				ofa_req_build => ["opensm"],
				ofa_req_inst => ["opensm"],
				},
			'libopensm-devel' =>
				{
				parent => "opensm",
				dist_req_build => ["bison", "flex"],
				dist_req_inst => ["bison", "flex"],
				ofa_req_inst => ["opensm", "libopensm"],
				},
			'libopensm' =>
				{
				parent => "opensm",
				dist_req_build => ["bison", "flex"],
				dist_req_inst => ["bison", "flex"],
				ofa_req_build => ["libibumad-dev"],
				ofa_req_inst => ["libibumad3"],
				},
			'perftest' =>
				{
				dist_req_build => ["libpci-dev"],
				ofa_req_build => ["libibverbs-dev", "librdmacm-dev", "libibumad-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1", "libibumad3", "ibverbs-providers"],
				},
			'srptools' =>
				{
				parent => "rdma-core",
				ofa_req_build => ["librdmacm-dev", "libibverbs-dev", "libibumad-dev"],
				ofa_req_inst => ["librdmacm1", "libibumad3", "libibverbs1", "rdma-core"],
				},

			'iser' =>
				{
				parent => "iser",
				available => 0,
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				},
			'iser-dkms' =>
				{
				parent => "iser",
				mode => "kernel",
				dist_req_build => ["dkms", "gcc", "make"],
				dist_req_inst => ["dkms", "gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},
			'iser-modules' =>
				{
				parent => "iser",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},

			'isert' =>
				{
				parent => "isert",
				available => 0,
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				},
			'isert-dkms' =>
				{
				parent => "isert",
				available => 1,
				mode => "kernel",
				dist_req_build => ["dkms", "gcc", "make"],
				dist_req_inst => ["dkms", "gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},
			'isert-modules' =>
				{
				parent => "isert",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},

			'srp' =>
				{
				parent => "srp",
				available => 0,
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				},
			'srp-dkms' =>
				{
				parent => "srp",
				mode => "kernel",
				dist_req_build => ["dkms", "gcc", "make"],
				dist_req_inst => ["dkms", "gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},
			'srp-modules' =>
				{
				parent => "srp",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},

			'mlnx-nfsrdma' =>
				{
				parent => "mlnx-nfsrdma",
				available => 0,
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				},
			'mlnx-nfsrdma-dkms' =>
				{
				parent => "mlnx-nfsrdma",
				mode => "kernel",
				dist_req_build => ["dkms", "gcc", "make"],
				dist_req_inst => ["dkms", "gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},
			'mlnx-nfsrdma-modules' =>
				{
				parent => "mlnx-nfsrdma",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},

			'mlnx-nvme' =>
				{
				parent => "mlnx-nvme",
				available => 0,
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				},
			'mlnx-nvme-dkms' =>
				{
				parent => "mlnx-nvme",
				mode => "kernel",
				dist_req_build => ["dkms", "gcc", "make"],
				dist_req_inst => ["dkms", "gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},
			'mlnx-nvme-modules' =>
				{
				parent => "mlnx-nvme",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},

			'fwctl' =>
				{
				parent => "fwctl",
				available => 0,
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				},
			'fwctl-dkms' =>
				{
				parent => "fwctl",
				mode => "kernel",
				dist_req_build => ["dkms", "gcc", "make"],
				dist_req_inst => ["dkms", "gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},
			'fwctl-modules' =>
				{
				parent => "fwctl",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},

                        'virtiofs' =>
                                {
                                parent => "virtiofs",
                                available => 0,
                                mode => "kernel",
                                dist_req_build => ["gcc", "make"],
                                dist_req_inst => ["gcc", "make"],
                                },
                        'virtiofs-dkms' =>
                                {
                                parent => "virtiofs",
                                mode => "kernel",
                                dist_req_build => ["dkms", "gcc", "make"],
                                dist_req_inst => ["dkms", "gcc", "make"],
                                ofa_req_build => ["$mlnX_ofed_kernel"],
                                ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
                                },
                        'virtiofs-modules' =>
                                {
                                parent => "virtiofs",
                                mode => "kernel",
                                dist_req_build => ["gcc", "make"],
                                dist_req_inst => ["gcc", "make"],
                                ofa_req_build => ["$mlnX_ofed_kernel"],
                                ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
                                },

			'mlnx-rdma-rxe' =>
				{
				parent => "mlnx-rdma-rxe",
				available => 0,
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				},
			'mlnx-rdma-rxe-dkms' =>
				{
				parent => "mlnx-rdma-rxe",
				mode => "kernel",
				dist_req_build => ["dkms", "gcc", "make"],
				dist_req_inst => ["dkms", "gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},
			'mlnx-rdma-rxe-modules' =>
				{
				parent => "mlnx-rdma-rxe",
				mode => "kernel",
				dist_req_build => ["gcc", "make"],
				dist_req_inst => ["gcc", "make"],
				ofa_req_build => ["$mlnX_ofed_kernel"],
				ofa_req_inst => ["ofed-scripts","$mlnX_ofed_kernel","mlnx-ofed-kernel-utils"],
				},

			'nvme-snap' =>
				{
				available => 0,
				ofa_req_build => ["libibverbs-dev", "librdmacm-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1", "spdk"],
				},

			'mlxbf-bootctl' =>
				{
				available => 0,
				},

			'ibdump' =>
				{
				available => 1,
				dist_req_build => ["libstdc++6"],
				dist_req_inst => ["libstdc++6"],
				ofa_req_build => ["libibverbs-dev"],
				ofa_req_inst => ["libibverbs1"],
				},

			'spdk' =>
				{
				parent=> "spdk",
				available => 0,
				dist_req_build => ["libiscsi-dev", "libaio-dev", "libcunit1-dev", "libncurses-dev"],
				dist_req_inst => ["libiscsi7", "libaio1", "libcunit1", "libncurses5"],
				ofa_req_build => ["libibverbs-dev", "librdmacm-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1"],
				},

			'spdk-dev' =>
				{
				parent=> "spdk",
				available => 0,
				dist_req_build => ["libiscsi-dev", "libaio-dev", "libcunit1-dev", "libncurses-dev"],
				dist_req_inst => ["libiscsi7", "libaio1", "libcunit1", "libncurses5"],
				ofa_req_build => ["libibverbs-dev", "librdmacm-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1", "spdk"],
				},

			'mlnx-ethtool' =>
				{
				dist_req_build => ["pkg-config", "libmnl-dev"],
				dist_req_inst => ["libmnl0"],
				},

			'mlnx-iproute2' =>
				{
				dist_req_build => ["libelf-dev", "libselinux1-dev", "libdb-dev", "libmnl-dev", "sudo", "bison", "flex"],
				dist_req_inst => ["$libelf", "libselinux1", "libmnl0"],
				},

			# this package is listed here only for uninstall and --without.. flag support
			'mlnx-fw-updater' =>
				{
				},

			'rdma-core' =>
				{
				parent => "rdma-core",
				dist_req_build => [
					"cmake",
					@rdmacore_python,
					$dh_systemd,
					"dh-python",
					"libnl-3-dev", "libnl-route-3-dev",
					@libsystemd_dev,
					"libudev-dev",
					"pandoc",
					"pkg-config",
					"python3-docutils",
					"$valgrind"
				],
				dist_req_inst => ["lsb-base", "libnl-3-200", "udev", "$libudev"],
				ofa_req_build => [],
				ofa_req_inst => ["ofed-scripts"],
				},
			'ibverbs-providers' =>
				{
				parent => "rdma-core",
				dist_req_build => ["libnuma-dev"],
				dist_req_inst => ["libnuma1"],
				ofa_req_build => ["rdma-core"],
				ofa_req_inst => ["rdma-core"],
				},
			'libibumad3' =>
				{
				parent => "rdma-core",
				dist_req_build => ["libtool"],
				ofa_req_build => ["rdma-core"],
				ofa_req_inst => ["rdma-core"],
				},
			'libibumad-dev' =>
				{
				parent => "rdma-core",
				dist_req_build => ["libtool"],
				ofa_req_build => ["rdma-core"],
				ofa_req_inst => ["libibumad3", "rdma-core"],
				},
			'rdmacm-utils' =>
				{
				parent => "rdma-core",
				ofa_req_build => ["rdma-core"],
				ofa_req_inst => ["librdmacm1", "rdma-core"],
				},
			'python3-pyverbs' =>
				{
				parent => "rdma-core",
				dist_req_inst => ["python3", "$libpython"],
				ofa_req_build => ["rdma-core"],
				ofa_req_inst => ["rdma-core", "libibverbs1", "librdmacm1", "ibverbs-providers"],
				},
			'mlx-steering-dump' =>
				{
				available => 0,
				dist_req_build => ["python3"],
				dist_req_inst => ["python3"],
				},
			'mlnx-dpdk' =>
				{
				parent => "mlnx-dpdk",
				do_not_release => 1,
				dist_req_build => [
					"dh-python", "libbsd-dev", "libcap-dev", "libelf-dev",
					"$libipsec_mb_dev", "libjansson-dev", "libmnl-dev",
					"libnuma-dev", "libpcap-dev", "libssl-dev", "meson",
					"python3", "zlib1g-dev", "ninja-build",
					"$linux_headers", "python3-pyelftools",
				],
				dist_req_inst => [
					"hwdata", "lsb-base", "pciutils", "libbsd0", "$libelf",
					"libjansson4", "libnuma1", "$libpcap", "zlib1g"
				],
				ofa_req_build => ["libibverbs-dev"],
				ofa_req_inst => ["libibverbs1"],
				},
			'mlnx-dpdk-dev' =>
				{
				parent => "mlnx-dpdk",
				do_not_release => 1,
				dist_req_build => [
					"dh-python", "libbsd-dev", "libcap-dev", "libelf-dev",
					"$libipsec_mb_dev", "libjansson-dev", "libmnl-dev",
					"libnuma-dev", "libpcap-dev", "libssl-dev", "meson",
					"python3", "zlib1g-dev", "ninja-build",
					"$linux_headers",
				],
				dist_req_inst => [
					"libbsd-dev", "libmnl-dev", "libnuma-dev",
					"libpcap-dev", "libssl-dev", "zlib1g-dev"
				],
				ofa_req_build => ["libibverbs-dev"],
				ofa_req_inst => ["libibverbs1", "mlnx-dpdk"],
				},
			'mlnx-dpdk-doc' =>
				{
				parent => "mlnx-dpdk",
				available => 0,
				do_not_release => 1,
				dist_req_build => ["libcap-dev", "libpcap-dev", "libnuma-dev", "python3", "python3-sphinx"],
				ofa_req_build => ["libibverbs-dev", "librdmacm-dev"],
				ofa_req_inst => ["libibverbs1", "librdmacm1"],
				},
			'ibarr' =>
				{
				dist_req_build => ["libnl-3-dev", "libnl-route-3-dev", "cmake", "pkg-config"],
				dist_req_inst => ["libnl-3-200"],
				ofa_req_build => ["libibverbs-dev", "libibumad-dev"],
				ofa_req_inst => ["libibverbs1", "libibumad3"],
				},
);

foreach my $package (keys %packages_info) {
	for my $key (qw/name parent/) {
		if (not exists $packages_info{$package}{$key}) {
			$packages_info{$package}{$key} = $package;
		}
	}
	for my $key (qw/selected installed deb_exist/) {
		if (not exists $packages_info{$package}{$key}) {
			$packages_info{$package}{$key} = 0;
		}
	}
	if (not exists $packages_info{$package}{'available'}) {
		$packages_info{$package}{'available'} = 1;
	}
	if (not exists $packages_info{$package}{'mode'}) {
		$packages_info{$package}{'mode'} = "user";
	}
	for my $key (qw/dist_req_build dist_req_inst ofa_req_build ofa_req_inst/) {
		if (not exists $packages_info{$package}{$key}) {
			$packages_info{$package}{$key} = [];
		}
	}
	if (not exists $packages_info{$package}{'configure_options'}) {
		$packages_info{$package}{'configure_options'} = '';
	}
}

for my $ovsp (@openvswitch_packages) {
	$packages_info{$ovsp}{'name'} = $ovsp;
	$packages_info{$ovsp}{'parent'} = "openvswitch";
	$packages_info{$ovsp}{'selected'} = 0;
	$packages_info{$ovsp}{'installed'} = 0;
	$packages_info{$ovsp}{'deb_exist'} = 0;
	$packages_info{$ovsp}{'available'} = 0;
	$packages_info{$ovsp}{'mode'} = 'user';
	$packages_info{$ovsp}{'dist_req_build'} = [
		"openssl", "libssl-dev", "graphviz", "dh-autoreconf",
		"procps", "python3-all", "libunwind-dev",
		"python3-zope.interface", "python3-sphinx",
		"libunbound-dev", "python3-six", "python3-twisted",
		"python3-prometheus-client", "libdbus-1-dev", "python3-setuptools",
		"dh-exec", "libcap-ng-dev", "python3-all-dev", "python3-sortedcontainers",
		"bc", "libacl1-dev", "libbz2-dev", "liblz4-dev", "libpcap-dev", "libxml2-dev", "libzstd-dev", "nettle-dev"
	];
	$packages_info{$ovsp}{'dist_req_inst'} = [];
	$packages_info{$ovsp}{'ofa_req_build'} = [];
	$packages_info{$ovsp}{'ofa_req_inst'} = [];
	$packages_info{$ovsp}{'configure_options'} = '';
}

if (exists $package_pre_build_script{'ibdump'} and $package_pre_build_script{'ibdump'} =~ /mft/) {
	$packages_info{'ibdump'}{'dist_req_inst'} = ["dkms"];
}

if ($distro =~ /uos/ and $arch eq "aarch64") {
	foreach (qw/iser-modules mlnx-nvme-modules/) {
		$packages_info{"$_"}{'available'} = 0;
	}
}

$packages_info{"openvswitch-common"}{'available'} = 1;
$packages_info{"openvswitch-common"}{'dist_req_inst'} = ["libbsd0", "libnuma1", "$libssl", "libunbound8", "libunwind8"];

$packages_info{"openvswitch-dev"}{'available'} = 1;
$packages_info{"openvswitch-dev"}{'dist_req_inst'} = ["$libssl_devel", "libunbound-dev"];

$packages_info{"openvswitch-switch"}{'available'} = 1;
$packages_info{"openvswitch-switch"}{'ofa_req_inst'} = ["openvswitch-common", "python3-openvswitch", "libibverbs1", "ibverbs-providers"];
$packages_info{"openvswitch-switch"}{'dist_req_inst'} = ["python3", "python3-netifaces", "netbase", "procps", "uuid-runtime", "$libpcap", "libjansson4", "$libipsec_mb", "ifupdown"];

$packages_info{"openvswitch-ipsec"}{'available'} = 1;
$packages_info{"openvswitch-ipsec"}{'ofa_req_inst'} = ["openvswitch-common", "openvswitch-switch", "python3-openvswitch"];
$packages_info{"openvswitch-ipsec"}{'dist_req_inst'} = ["python3", "strongswan", "strongswan-swanctl"];

$packages_info{"openvswitch-pki"}{'available'} = 1;
$packages_info{"openvswitch-pki"}{'ofa_req_inst'} = ["openvswitch-common"];

$packages_info{"openvswitch-test"}{'available'} = 1;
$packages_info{"openvswitch-test"}{'dist_req_inst'} = ["python3-twisted"];

$packages_info{"openvswitch-vtep"}{'available'} = 1;
$packages_info{"openvswitch-vtep"}{'ofa_req_inst'} = ["openvswitch-common", "openvswitch-switch", "python3-openvswitch"];

$packages_info{"python3-openvswitch"}{'available'} = 1;
$packages_info{"python3-openvswitch"}{'dist_req_inst'} = ["python3", "python3-six"];

if ($rdmacore_python[0] eq "python") {
	$packages_info{"python3-pyverbs"}{"available"} = 0;
}

if ($distro =~ /ubuntu24.10/) {
	$packages_info{"ucx"}{'available'} = 0;
	$kernel_modules_info{'nfsrdma'}{'available'} = 0;
	$packages_info{"$mlnx_nfsrdma"}{'available'} = 0;
}

if ($distro =~ /ubuntu26.04/) {
	$packages_info{"openmpi"}{'available'} = 0;
}

if ($distro =~ /velinux/ and $arch eq "aarch64") {
	block_package("openmpi");
}

$packages_info{"libibverbs-dev"}{'dist_req_inst'} = ['libnl-3-dev', 'libnl-route-3-dev'];
$packages_info{"infiniband-diags"}{'dist_req_build'} = [''];
$packages_info{"infiniband-diags"}{'dist_req_inst'} = [''];

my $components_filter = get_components_filter(1, [@components]);

###############

sub getch
{
	my $c;
	system("stty -echo raw");
	$c=getc(STDIN);
	system("stty echo -raw");
	# Exit on Ctrl+c or Esc
	if ($c eq "\cC" or $c eq "\e") {
		print "\n";
		exit 1;
	}
	print "$c\n";
	return $c;
}

sub is_installed_deb
{
	my $name = shift @_;

	my $installed_deb = `$DPKG_QUERY -l $name 2> /dev/null | awk '/^[rhi][iU]/{print \$2}'`;

	return ($installed_deb) ? 1 : 0;
}

sub get_all_matching_installed_debs
{
	my $name = shift @_;

	my $installed_debs = `dpkg-query -l "*$name*" 2> /dev/null | awk '/^[rhi][iU]/{print \$2}' | sed -e 's/:.*//g'`;
	$installed_debs =~ s/libarra\S*// if ($name eq 'ibarr');
	return (split "\n", $installed_debs);
}

my %check_uninstall = ();
my %purge_no_deps = ();

# Removes a potential ':<archname>' suffix. e.g.
# 'package:amd64' -> 'package'
sub strip_package_arch($) {
	my $package = shift;
	$package =~ s/:.*//;
	return $package;
}

sub is_immuned($) {
	my $package = shift;
	$package = strip_package_arch($package);
	return exists $purge_no_deps{$package};
}

sub set_immuned($) {
	my $package = shift;
	$package = strip_package_arch($package);
	$purge_no_deps{$package} = 1;
}

sub mark_for_uninstall
{
	my $package = shift @_;

	return if ($package =~ /^xen|ovsvf-config|opensmtpd/);
	return if (is_immuned($package));

	if (not $selected_for_uninstall{$package}) {
		if (is_installed_deb $package) {
			print_and_log("$package will be removed.\n", $verbose2);
			push (@dependant_packages_to_uninstall, "$package");
			$selected_for_uninstall{$package} = 1;
			if (not (exists $packages_info{$package} or $package =~ /mlnx-ofed- | doca-/x)) {
				$non_ofed_for_uninstall{$package} = 1;
			}
		}
	}
}

sub get_requires
{
	my $package = shift @_;

	chomp $package;

	if ($check_uninstall{$package}) {
		return; # already checked here
	}
	$check_uninstall{$package} = 1;

	if ($package eq "rdma") {
		# don't remove packages that needs rdma package
		return;
	}

	my @what_requires = `/usr/bin/dpkg --purge --dry-run '$package' 2>&1 | grep "depends on" 2> /dev/null`;

	for my $pack_req (@what_requires) {
		chomp $pack_req;
		$pack_req =~ s/\s*(.+) depends.*/$1/g;
		$pack_req =~ s/:.*//g;
		if (exists $immune_debs{$pack_req}) {
			print_and_log("get_requires: $package is required by $pack_req, but $pack_req won't be removed.\n", $verbose);
			set_immuned($package);
			$check_uninstall{$pack_req} = 1;
			return;
		}
		print_and_log("get_requires: $package is required by $pack_req\n", $verbose2);
		get_requires($pack_req);
		mark_for_uninstall($pack_req);
	}
}

sub is_configured_deb
{
	my $name = shift @_;

	my $installed_deb = `$DPKG_QUERY -l $name 2> /dev/null | awk '/^rc/{print \$2}'`;
	return ($installed_deb) ? 1 : 0;
}

sub ex
{
	my $cmd = shift @_;
	my $sig;
	my $res;

	print_and_log("Running: $cmd\n", $verbose2);
	system("$cmd >> '$glog' 2>&1");
	$res = $? >> 8;
	$sig = $? & 127;
	if ($sig or $res) {
		print_and_log_colored("Failed command: $cmd", 1, "RED");
		print_and_log("See $glog", 1);
		exit 1;
	}
}

sub ex_deb_build
{
	my $name = shift @_;
	my $cmd = shift @_;
	my $sig;
	my $res;

	print_and_log("Running $cmd\n", $verbose);
	system("echo $cmd > $ofedlogs/$name.debbuild.log 2>&1");
	system("$cmd >> $ofedlogs/$name.debbuild.log 2>&1");
	$res = $? >> 8;
	$sig = $? & 127;
	if ($sig or $res) {
		print_and_log_colored("Failed to build $name DEB", 1, "RED");
		addSetupInfo ("$ofedlogs/$name.debbuild.log");
		print_and_log_colored("See $ofedlogs/$name.debbuild.log", 1, "RED");
		exit 1;
	}
}

sub ex_deb_install
{
	my $name = shift @_;
	my $cmd = shift @_;
	my $sig;
	my $res;

	return 0 if ($build_only);

	if (exists $package_pre_install_script{$name}) {
		print_and_log("Running $name pre install script: $package_pre_install_script{$name}\n", $verbose);
		ex1("$package_pre_install_script{$name}");
	}

	print_and_log("Running $cmd\n", $verbose);
	system("echo $cmd > $ofedlogs/$name.debinstall.log 2>&1");
	system("$cmd >> $ofedlogs/$name.debinstall.log 2>&1");
	$res = $? >> 8;
	$sig = $? & 127;
	if ($sig or $res) {
		print_and_log_colored("Failed to install $name DEB", 1, "RED");
		addSetupInfo ("$ofedlogs/$name.debinstall.log");
		print_and_log_colored("See $ofedlogs/$name.debinstall.log", 1, "RED");
		if ($name =~ /-dkms$/) {
			copy_make_log($name);
		}
		exit 1;
	}

	if (exists $package_post_install_script{$name}) {
		print_and_log("Running $name post install script: $package_post_install_script{$name}\n", $verbose);
		ex1("$package_post_install_script{$name}");
	}
}

sub check_linux_dependencies
{
	my $kernel_dev_missing = 0;
	my %missing_packages = ();

	if (! $check_linux_deps) {
		return 0;
	}

	print_and_log("Checking SW Requirements...\n", (not $quiet));
	foreach my $req_name (@required_debs) {
		my $is_installed_flag = is_installed_deb($req_name);
		if (not $is_installed_flag) {
			print_and_log_colored("$req_name deb is required", $verbose2, "RED");
			$missing_packages{"$req_name"} = 1;
		}
	}

	foreach my $package (@selected_packages) {
		my $pname = $packages_info{$package}{'parent'};
		for my $ver (keys %{$main_packages{$pname}}) {
			if ($package =~ /kernel|knem|mlnx-en|^xpmem/) {
				# kernel sources are required to build mlnx-ofed-kernel
				# require only if with_dkms=1 or (with_dkms=0 and deb is not built)
				if ( not -d "$kernel_sources/scripts" and
						($with_dkms or (not $with_dkms and not is_deb_available("$package")))) {
					print_and_log_colored("$kernel_sources/scripts is required to build $package package.", $verbose2, "RED");
					if (not $kernel_given and $distro =~ /debian/) {
						my $debian_arch = `dpkg-architecture -qDEB_TARGET_ARCH`;
						chomp $debian_arch;
						$kernel =~ s|-[^-]+$|-$debian_arch|;
					}
					$missing_packages{"linux-headers-$kernel"} = 1;
					$kernel_dev_missing = 1;
				}
				# from kernel 4.14 we need elf devel package when CONFIG_UNWINDER_ORC=y
				if ( not is_installed_deb($kernel_elfutils_devel) and check_autofconf('CONFIG_STACK_VALIDATION') eq "1" and check_autofconf('CONFIG_UNWINDER_ORC') eq "1" and $kernel =~ /^[5-9]|^4\.[1-9][4-9]\./) {
                                $missing_packages{"$kernel_elfutils_devel"} = 1;
                                print_and_log_colored("$kernel_elfutils_devel is required to build $package RPM.", $verbose2, "RED");
                            }
			}

			# Check rpmbuild requirements
			if (not $packages_info{$package}{$ver}{'deb_exist'}) {
				for my $req ( @{ $packages_info{$package}{'dist_req_build'} } ) {
					next if not $req;
					if (not is_installed_deb($req)) {
						print_and_log_colored("$req deb is required to build $package $ver", $verbose2, "RED");
						$missing_packages{"$req"} = 1;
					}
				}
			}

			# Check installation requirements
			for my $req_name ( @{ $packages_info{$package}{'dist_req_inst'} } ) {
                                next if not $req_name;
				last if $build_only;
				my $is_installed_flag = is_installed_deb($req_name);
				if (not $is_installed_flag) {
					print_and_log("$req_name deb is required to install $package $ver", $verbose2, "RED");
					$missing_packages{"$req_name"} = 1;
				}
			}
		}
	}

	# display a summary of missing packages
	if (keys %missing_packages) {
		print_and_log_colored("One or more required packages for installing OFED-internal are missing.", 1, "RED");
		if ($kernel_dev_missing) {
			print_and_log_colored("$kernel_sources/scripts is required for the Installation.", 1, "RED");
		}
		if ($check_deps_only) {
			print_and_log("Run:\napt-get install " . join(' ', (keys %missing_packages)) . "\n", 1);
			exit $PREREQUISIT;
		} else {
			purge_failed_dkms_packages();
			print_and_log_colored("Attempting to install the following missing packages:\n" . join(' ', (keys %missing_packages)), 1, "RED");
			my $cmd = "apt-get install -y $apt_extra_params " . join(' ', (keys %missing_packages));
			print_and_log("Running: apt-get update\n", $verbose2);
			system("apt-get update >> $glog 2>&1");
			ex "$cmd";
		}
	}

	if ($check_deps_only) {
		print_and_log("All required packages are installed, the system is ready for $PACKAGE installation.\n", 1);
		exit 0;
	}
}

sub get_module_list_from_dkmsConf
{
	my $conf = shift;

	my @modules = ();
	open(IN, "$conf") or print_and_log_colored("Error: cannot open file: $conf", 1, "RED");
	while(<IN>) {
		my $mod = $_;
		chomp $mod;
		if ($mod =~ /BUILT_MODULE_NAME/) {
			$mod =~ s/BUILT_MODULE_NAME\[[0-9]*\]=//g;
			$mod =~ s@^ib_@@g;
			if ($mod =~ /eth_ipoib/) {
				$mod =~ s/eth_ipoib/e_ipoib/g;
			}
			push(@modules, $mod);
		}
	}
	close(IN);
	return @modules;
}

sub is_module_in_deb
{
	my $name = shift;
	my $module = shift;

	my $ret = 0;
	my $deb = "";
	if ($name =~ /mlnx-ofed-kernel/) {
		($deb) = glob ("$DEBS/$mlnX_ofed_kernel*.deb");
	} elsif ($name =~ /mlnx-en/) {
		($deb) = glob ("$DEBS/$mlnx_en*.deb");
	} else {
		($deb) = glob ("$DEBS/$name*.deb");
	}

	if ($deb) {
		if ($module =~ /srp|iser|sdp|rds|nfsrdma|mlnx-nvme|mlnx-rdma-rxe|fwctl|virtiofs/) {
			return 1;
		}
		rmtree "$builddir/$name\_module-check";
		mkpath "$builddir/$name\_module-check";
		ex "$DPKG_DEB -x '$deb' $builddir/$name\_module-check 2>/dev/null";
		if (basename($deb) =~ /dkms/) {
			my $conf = `find $builddir/$name\_module-check -name dkms.conf 2>/dev/null | grep -vE "drivers/|net/|fs/"`;
			chomp $conf;
			if (grep( /$module.*$/, get_module_list_from_dkmsConf($conf))) {
				print_and_log("is_module_in_deb: $module is in $deb\n", $verbose2);
				$ret = 1;
			} else {
				print_and_log("is_module_in_deb: $module is NOT in $deb\n", $verbose2);
				$ret = 0;
			}
		} else {
			my $modpath = `find $builddir/$name\_module-check -name "*${module}*.ko" 2>/dev/null`;
			chomp $modpath;
			if ($modpath ne "") {
				print_and_log("is_module_in_deb: $module is in $deb\n", $verbose2);
				$ret = 1;
			} else {
				print_and_log("is_module_in_deb: $module is NOT in $deb\n", $verbose2);
				$ret = 0;
			}
		}
		rmtree "$builddir/$name\_module-check";
	} else {
		print_and_log("deb file was not found for pacakge: $name\n", $verbose2);
	}

	return $ret;
}

#
# print usage message
#
sub usage
{
   print GREEN;
   print "\n";
   print "Usage: $0 [-c <packages config_file>|--all|--hpc|--vma|--xlio|--basic|--bluefield] [OPTIONS]\n";

   print "\n";
   print "Installation control:\n";
   print "    --force              Force installation\n";
   print "    --tmpdir             Change tmp directory. Default: $TMPDIR\n";
   print "    -k|--kernel <version>\n";
   print "                         Default on this system: $kernel (relevant if --without-dkms is given)\n";
   print "    -s|--kernel-sources <path>\n";
   print "                         Default on this system: $kernel_sources (relevant if --without-dkms is given)\n";
   print "    -b|--build-only      Build binary DEBs without installing them (relevant if --without-dkms is given)\n";
   print "                         - This option is supported only when '--kernel-only' option is given.\n";
   print "    --distro             Set Distro name for the running OS (e.g: ubuntu14.04). Default: Use auto-detection.\n";
   print "    --without-depcheck   Run the installation without verifying that all required Distro's packages are installed\n";
   print "    --check-deps-only    Check for missing required Distro's packages and exit\n";
   print "    --print-distro       Print distribution name and exit\n";
   print "    --force-dkms         Force installing kernel packages with DKMS support\n";
   print "    --without-dkms       Don't install kernel packages with DKMS support\n";
   print "    --without-dkms-install    Build DKMS packages, but don't install them.\n";
   print "    --builddir           Change build directory. Default: $builddir\n";
   print "    --umad-dev-rw        Grant non root users read/write permission for umad devices instead of default\n";
   print "    --umad-dev-na        Prevent from non root users read/write access for umad devices. Overrides '--umad-dev-rw'\n";
   print "    --enable-mlnx_tune   Enable Running the mlnx_tune utility\n";
   print "    --enable-opensm      Run opensm upon boot\n";
   print "    --without-mlx5-ipsec Disable IPsec support on ConnectX adapters\n";
   print "\n";
   print "    --package-install-options\n";
   print "                         DPKG install options to use when installing DEB packages (comma separated list)\n";
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
   print "    --xlio            Install minimum packages required by XLIO\n";
if (not $install_option eq 'eth-only') {
   print "    --guest              Install minimum packages required by guest OS\n";
   print "    --hypervisor         Install minimum packages required by hypervisor OS\n";
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
   print "    --with-memtrack      Build ofa_kernel deb with memory tracking enabled for debugging\n";
   print "    --kernel-extra-args '<args>'\n";
   print "                         pass <args> to kernel configure script (single paraeter, space separated)\n";
   print "\n";
   print "Miscellaneous:\n";
   print "    -h|--help            Display this help message and exit\n";
   print "    -p|--print-available Print available packages for current platform\n";
   print "                         And create corresponding ofed.conf file\n";
   print "    --conf-dir           Destination directory to save the configuration file. Default: $CWD\n";
   print "    --copy-ifnames-udev  Copy compatibility udev rules for interface names\n";
   print "\n";
   print "Output control:\n";
   print "    -v|-vv|-vvv          Set verbosity level\n";
   print "    -q                   Set quiet - no messages will be printed\n";
   print RESET "\n\n";
}

sub count_ports
{
	my $cnt = 0;
	open(LSPCI, "/usr/bin/lspci -n|");

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

# removes the settings for a given interface from /etc/network/interfaces
sub remove_interface_settings
{
	my $interface = shift @_;

	open(IFCONF, $ifconf) or die "Can't open $ifconf: $!";
	my @ifconf_lines;
	while (<IFCONF>) {
		push @ifconf_lines, $_;
	}
	close(IFCONF);

	open(IFCONF, ">$ifconf") or die "Can't open $ifconf: $!";
	my $remove = 0;
	foreach my $line (@ifconf_lines) {
		chomp $line;
		if ($line =~ /(iface|mapping|auto|allow-|source) $interface/) {
			$remove = 1;
		}
		if ($remove and $line =~ /(iface|mapping|auto|allow-|source)/ and $line !~ /$interface/) {
			$remove = 0;
		}
		next if ($remove);
		print IFCONF "$line\n";
	}
	close(IFCONF);
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
	my $ret;
	my $ip;
	my $nm;
	my $nw;
	my $bc;
	my $onboot = 1;
	my $found_eth_up = 0;
	my $eth_dev;

	if (not $config_net_given) {
		return;
	}
	print "Going to update $dev in $ifconf\n" if ($verbose2);
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
		get_net_config($eth_dev, \%ifcfg, '');
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
	if ($onboot) {
		print "auto $dev\n";
	}
	print "iface $dev inet static\n";
	print "address $ip\n";
	print "netmask $nm\n";
	print "network $nw\n";
	print "broadcast $bc\n";
	print RESET "\n";

	# Remove old interface's settings
	remove_interface_settings($dev);

	# append the new interface's settings to the interfaces file
	open(IF, ">>$ifconf") or die "Can't open $ifconf: $!";
	print IF "\n";
	if ($onboot) {
		print IF "auto $dev\n";
	}
	print IF "iface $dev inet static\n";
	print IF "\taddress $ip\n";
	print IF "\tnetmask $nm\n";
	print IF "\tnetwork $nw\n";
	print IF "\tbroadcast $bc\n";
	close(IF);
}

sub ipoib_config
{
	if (not $config_net_given) {
		return;
	}

	my $ports_num = count_ports();
	for (my $i = 0; $i < $ports_num; $i++ ) {
		config_interface($i);
	}
}

sub get_tarball_available
{
	my $name = shift;

	for my $ver (keys %{$main_packages{$name}}) {
		if ($main_packages{$name}{$ver}{'tarballpath'}) {
			return $main_packages{$name}{$ver}{'tarballpath'};
		}
	}

	return "";
}

sub is_tarball_available
{
	my $name = shift;

	for my $ver (keys %{$main_packages{$name}}) {
		if ($main_packages{$name}{$ver}{'tarballpath'}) {
			return 1;
		}
	}

	return 0;
}

sub is_deb_available
{
	my $name = shift;
	for my $ver (keys %{$main_packages{$name}}) {
		if ($main_packages{$name}{$ver}{'debpath'}) {
			return 1;
		}
	}

	return 0;
}

sub add_packages($$@) {
	my $fh = shift;
	my $cnt = shift;
	for my $package (@_) {
		next if (grep /^$package$/, @selected_by_user);
		next if (not $packages_info{$package}{'available'});
		my $parent = $packages_info{$package}{'parent'};
		next if (not is_tarball_available($parent));
		push (@selected_by_user, $package);
		print $fh "$package=y\n";
		$cnt ++;
	}
	return $cnt;
}

sub get_uninstall_env_str() {
    my $components_str = 'MLNX_OFED_COMPONENTS="'. join(" ", @components) . '"';
    return "$components_str";
}

# select packages to install
sub select_packages
{
	my $cnt = 0;
	if ($config_given) {
		open(CONFIG, "$config") || die "Can't open $config: $!";;
		while(<CONFIG>) {
			next if (m@^\s+$|^#.*@);
			my ($package,$selected) = (split '=', $_);
			chomp $package;
			chomp $selected;

			# fix kernel packages names
			# DKMS enabled
			if ($with_dkms) {
				if ($package =~ /-modules/) {
					$package =~ s/-modules/-dkms/g;;
				}
			} else {
			# DKMS disabled
				if ($package =~ /-dkms/) {
					$package =~ s/-dkms/-modules/g;;
				}
			}

			print_and_log("$package=$selected\n", $verbose2);

			if (not $packages_info{$package}{'parent'} or $package =~ /iser|srp$/) {
				my $modules = "@kernel_modules";
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
				if (not is_tarball_available($parent)) {
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
		$config = $conf_dir . "/ofed-$install_option.conf";
		chomp $config;
		open(CONFIG, ">$config") || die "Can't open $config: $!";
		flock CONFIG, $LOCK_EXCLUSIVE;
		if ($install_option eq 'all') {
			for my $package ( @all_packages ) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
		elsif ($install_option eq 'bluefield') {
			for my $package ( @bluefield_packages ) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
			for my $package ( @hpc_packages ) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
				@list = (@vma_packages);
			} elsif ($install_option eq 'vmavpi') {
				@list = (@vmavpi_packages);
			} elsif ($install_option eq 'vmaeth') {
				@list = (@vmaeth_packages);
			}
			for my $package ( @list ) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
			for my $package (@xlio_packages) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
			for my $package (@basic_packages) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
		elsif ($install_option eq 'hypervisor') {
			for my $package ( @hypervisor_packages ) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
		elsif ($install_option eq 'guest') {
			for my $package ( @guest_packages ) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
		elsif ($install_option eq 'kernel-only') {
			for my $package ( @all_packages ) {
				next if (not $packages_info{$package}{'available'});
				next if (not $packages_info{$package}{'mode'} eq 'kernel');
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
			for my $package (@eth_packages) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
				push (@selected_by_user, $package);
				print CONFIG "$package=y\n";
				$cnt ++;
			}
			for my $module ( @eth_kernel_modules ) {
				next if (not $kernel_modules_info{$module}{'available'});
				push (@selected_modules_by_user, $module);
				print CONFIG "$module=y\n";
			}
		}
		elsif ($install_option =~ m/dpdk/) {
			for my $package ( @dpdk_packages ) {
				next if (not $packages_info{$package}{'available'});
				my $parent = $packages_info{$package}{'parent'};
				next if (not is_tarball_available($parent));
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
		$cnt = add_packages(*CONFIG, $cnt, @bluefield_packages);
		for my $module ( @bluefield_kernel_modules ) {
			next if (/^$module$/, @selected_modules_by_user);
			next if (not $kernel_modules_info{$module}{'available'});
			push (@selected_modules_by_user, $module);
			print CONFIG "$module=y\n";
		}
	}


	if ($with_openvswitch or $with_ovs) {
		$cnt = add_packages(*CONFIG, $cnt, @openvswitch_packages);
	}

	flock CONFIG, $UNLOCK;
	close(CONFIG);

	return $cnt;
}

# It should be possible for the user to pass extra build options
# from outside:
sub add_build_option($) {
	my $option = shift;

	if (exists $ENV{'DEB_BUILD_OPTIONS'}) {
		$ENV{'DEB_BUILD_OPTIONS'} = "$ENV{'DEB_BUILD_OPTIONS'} $option";
	} else {
		$ENV{'DEB_BUILD_OPTIONS'} = $option;
	}
}

# Reset DEB_BUILD_OPTIONS options between builds of different packages.
# Leave it as we got it:
sub reset_build_options(@) {
	return unless (exists $ENV{'DEB_BUILD_OPTIONS'});

	foreach my $option (@_) {
		$ENV{'DEB_BUILD_OPTIONS'} =~ s/$option//;
	}
	if ($ENV{'DEB_BUILD_OPTIONS'} =~ /^ *$/) {
		delete $ENV{'DEB_BUILD_OPTIONS'};
		return;
	}
	$ENV{'DEB_BUILD_OPTIONS'} =~ s/ +/ /g;
}

#
# install selected packages by the user (@selected_packages)
#
sub install_selected
{
	if ($build_only) {
		print_and_log("Installing new packages\n", 1);
	} else {
		print_and_log("Building packages\n", 1);
	}
	my $i = 0;

	chdir $CWD;
	foreach my $name (@selected_packages) {
		delete $ENV{"DEB_CONFIGURE_EXTRA_FLAGS"};
		delete $ENV{"configure_options"};
		delete $ENV{"PACKAGE_VERSION"};
		delete $ENV{"MPI_HOME"};
		delete $ENV{"KNEM_PATH"};
		delete $ENV{"DESTDIR"};
		delete $ENV{"libpath"};
		delete $ENV{"rdmascript"};
		delete $ENV{"CONFIG_ARGS"};
		delete $ENV{"WITH_DKMS"};
		delete $ENV{"MLNX_KO_NO_STRIP"};
		delete $ENV{"kernelver"};
		delete $ENV{"kernel_source_dir"};
		delete $ENV{"KVER"};
		delete $ENV{"K_BUILD"};
		delete $ENV{"MLX4"};
		delete $ENV{"MLX5"};
		delete $ENV{"MLXFW"};
		delete $ENV{"mellanox_autodetect"};
		$BUILD_ENV = '';
		reset_build_options('static', 'bluefield', 'nopyverbs');
		my $dpkg_params = '';

		my $parent = $packages_info{$name}{'parent'};
		my $deb_name = $packages_info{$name}{'name'};
		my $gz = get_tarball_available($parent);
		$gz =~ s/.*\/SOURCES/SOURCES/g;
		my $version = $gz;

		$version =~ s/^SOURCES\/(doca-)?${parent}_//;
		$version =~ s/(.orig).*//;
		my @debs = ();
		@debs = glob ("$DEBS/${deb_name}[-_]${version}*.deb");
		if ($name =~ /dapl/ or (not @debs and $name =~ /ucx|openmpi/)) {
			# TODO: this is neeeded only because of the bad version number in changelog
			@debs = glob ("$DEBS/${deb_name}[-_]*.deb");
		} elsif ($parent =~ /mlnx-ofed-kernel/) {
			if ($with_dkms) {
				@debs = glob ("$DEBS/${deb_name}_${version}-*.deb");
			} else {
				@debs = glob ("$DEBS/${deb_name}[-_]${version}-*.kver.${kernel}_*.deb");
				if (not @debs) {
					@debs = glob ("$DEBS/${deb_name}[-_]${version}-${kernel}_*.deb");
				}
				if (not @debs and $user_space_only) {
					# running user space only with non-dkms mode on a kernel that
					# we don't have bins for. so just take the dkms utils package
					@debs = glob ("$DEBS/${deb_name}[-_]${version}*OFED*.deb");
				}
			}
		} elsif (not $with_dkms and $parent =~ /iser|srp$|knem|kernel-mft|mlnx-en|mlnx-nfsrdma|mlnx-nvme|mlnx-rdma-rxe|fwctl|virtiofs|xpmem$/) {
			@debs = glob ("$DEBS/${deb_name}[-_]${version}-*.kver.${kernel}_*.deb");
			if (not @debs) {
				@debs = glob ("$DEBS/${deb_name}[-_]${version}-${kernel}_*.deb");
			}
		}

		if (not $gz and not @debs) {
			print_and_log("Tarball for $parent and DEBs for $name are missing\n", 1);
			next;
		}

		# check if selected modules are in the found deb file
		if (@debs and ($name =~ /$mlnX_ofed_kernel/) and ($name !~ /mlnx-ofed-kernel-dkms/)) {
			my $kernel_rpm = "$mlnX_ofed_kernel";
			my $pname = $packages_info{$kernel_rpm}{'parent'};
			for my $ver (keys %{$main_packages{$pname}}) {
				for my $module (@selected_kernel_modules) {
					if (not is_module_in_deb($kernel_rpm, "$module")) {
						@debs = ();
						last;
					}
				}
				if ($with_memtrack) {
					if (not is_module_in_deb($kernel_rpm, "memtrack")) {
						@debs = ();
						last;
					}
				}
			}
		}

		if (not @debs) {
			print_and_log("Building DEB for ${name}-${version} ($parent)...\n", (not $quiet), 1);
			# Build debs from source
			rmtree "$builddir/$parent";
			mkpath "$builddir/$parent";
			if ($parent =~ /openmpi/) {
				my $config_args = "$packages_info{'openmpi'}{'configure_options'} ";
				my $openmpiVer = glob ("SOURCES/openmpi_*gz");
				$openmpiVer =~ s/^SOURCES\/openmpi_//;
				$openmpiVer =~ s/(.orig).*//;
				$config_args .= " --prefix=/usr/mpi/gcc/openmpi-$openmpiVer";
				$config_args .= " --with-platform=contrib/platform/mellanox/optimized";
				$ENV{"CONFIG_ARGS"} = "$config_args";
				# Let openmpi automatically find plugins and use them
				$ENV{"mellanox_autodetect"} = "yes";
				$BUILD_ENV .= " mellanox_autodetect='yes'";
			} elsif ($parent =~ /openvswitch|mlnx-dpdk|mlnx-ethtool/) {
				if ($parent =~ /openvswitch|mlnx-ethtool/) {
					add_build_option('nocheck');
				}

				if ($parent =~ /mlnx-dpdk/) {
					$dpkg_params .= " -Pnodoc";
					if ($with_bluefield) {
						add_build_option('bluefield');
					}
				}
			} elsif ($parent =~ /mlnx-ofed-kernel/) {
				$kernel_configure_options = "";

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

						# Innova/FPGA IPsec supported only in MLNX_OFED, from kernel 4.13 and up
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
				if ($kernel_extra_args) {
					$kernel_configure_options .= " $kernel_extra_args";
				}

				$ENV{"configure_options"} = $kernel_configure_options;
				$ENV{"PACKAGE_VERSION"} = "$version";
			} elsif ($parent =~ /libxlio/x) {
				if ($packages_info{"dpcp"}{'selected'}) {
					$ENV{"configure_options"} = '--with-dpcp';
				}
			} elsif ($parent eq "ibsim") {
				$ENV{"libpath"} = "/usr/lib";
			} elsif ($parent eq "ucx") {
				if ($cuda_top_dir) {
					$dpkg_params .= " -Pcuda";
				}
			} elsif ($parent eq "mlnx-iproute2") {
				if ($with_bluefield) {
					$dpkg_params .= " -Pbluefield";
				}
			} elsif ($parent eq "rdma-core") {
				if (not $packages_info{"python3-pyverbs"}{"available"}) {
					add_build_option('nopyverbs');
				}
			} elsif ($parent =~ /mlnx-en/) {
				$ENV{"MLX4"} = (grep( /^mlx4$/, @selected_kernel_modules )) ? "1" : "0";
				$ENV{"MLX5"} = (grep( /^mlx5$/, @selected_kernel_modules )) ? "1" : "0";
				$ENV{"MLXFW"} = (grep( /^mlxfw$/, @selected_kernel_modules )) ? "1" : "0";
				$ENV{"MLNX_EN_KERNEL_MODS"} = join " ", @selected_kernel_modules;
			} elsif ($parent =~ /mstflint/) {
				$ENV{"DEB_CONFIGURE_EXTRA_FLAGS"} = '--disable-inband';
			} elsif ($parent eq "spdk") {
				# spdk depends on libiscsi-dev which depends on librdmacm1 and
				# therefore will be removed by the uninstall procedure.
				# So, reinstall SPDK dependencies here.
				my $cmd = "apt-get install -y $apt_extra_params " . join(' ', (@{ $packages_info{$parent}{'dist_req_build'} }));
				ex "$cmd";
			}

			chdir  "$builddir/$parent";
			ex "cp '$CWD/$gz' .";
			if ($gz =~ /\.tar\.xz$/) {
				ex "tar xJf '$CWD/$gz'";
			} else {
				ex "tar xzf '$CWD/$gz'";
			}

			my $extracted_dir = "$parent-$version";
			if ($gz =~ /doca-openvswitch/ && $parent eq "openvswitch") {
				$extracted_dir = "doca-$parent-$version";
			}

			chdir "$extracted_dir";

			if ($parent eq "openvswitch") {
				# Clean up configured files from tarball to avoid dpkg-source errors
				if (-f "Makefile" && -f "boot.sh") {
					ex "make maintainer-clean";
					ex "find python/ -name '*.pyc' -o -name '__pycache__' | xargs rm -rf";
				}
			}

			if (not $with_dkms and $parent =~ /mlnx-ofed-kernel|iser|srp$|knem|kernel-mft|mlnx-en|mlnx-nfsrdma|mlnx-nvme|mlnx-rdma-rxe|fwctl|virtiofs|xpmem$/) {
				$ENV{"WITH_DKMS"} = "0";
				$ENV{"kernelver"} = "$kernel";
				$ENV{"kernel_source_dir"} = "$kernel_sources";
				$ENV{"KVER"} = "$kernel";
				$ENV{"K_BUILD"} = "$kernel_sources";
				$dpkg_params .= " -Pmodules";
				if ($with_kmod_debug_symbols) {
					$ENV{"MLNX_KO_NO_STRIP"} = "1";
				}
				if (-f "debian/control.no_dkms") {
					ex "/bin/mv -f debian/control.no_dkms debian/control";
				}
			}

			if (exists $package_pre_build_script{$name}) {
				print_and_log("Running $name pre build script: $package_pre_build_script{$name}\n", $verbose);
				ex1("$package_pre_build_script{$name}");
			}

			ex_deb_build($parent, "$BUILD_ENV $DPKG_BUILDPACKAGE -us -uc $dpkg_params");
			ex "cp ../*.deb '$DEBS/'";

			my @built_debs = map {
				(my $foo = $_) =~ s@.*/@${DEBS}/@;
				$foo;
			} glob ("../*.deb");
			$packages_info{$name}{'built_debs'} = \@built_debs;

			if (exists $package_post_build_script{$name}) {
				print_and_log("Running $name post build script: $package_post_build_script{$name}\n", $verbose);
				ex1("$package_post_build_script{$name}");
			}

			@debs = glob ("$DEBS/${deb_name}[-_]${version}*.deb");
			if ($name =~ /dapl/ or (not @debs and $name =~ /ucx|openmpi/)) {
				# TODO: this is neeeded only because of the bad version number in changelog
				@debs = glob ("$DEBS/${deb_name}[-_]*.deb");
			} elsif ($parent =~ /mlnx-ofed-kernel/) {
				if ($with_dkms) {
					@debs = glob ("$DEBS/${deb_name}_${version}-*.deb");
				} else {
					@debs = glob ("$DEBS/${deb_name}[-_]${version}-*.kver.${kernel}_*.deb");
					if (not @debs) {
						@debs = glob ("$DEBS/${deb_name}[-_]${version}-${kernel}_*.deb");
					}
				}
			} elsif (not $with_dkms and $parent =~ /iser|srp$|knem|kernel-mft|mlnx-en|mlnx-nfsrdma|mlnx-nvme|mlnx-rdma-rxe|fwctl|virtiofs|xpmem$/) {
				@debs = glob ("$DEBS/${deb_name}[-_]${version}-*.kver.${kernel}_*.deb");
				if (not @debs) {
					@debs = glob ("$DEBS/${deb_name}[-_]${version}-${kernel}_*.deb");
				}
			}
			chdir $CWD;
			rmtree "$builddir/$parent";

			if (not @debs) {
				print_and_log_colored("Error: DEB for $name was not created !", 1, "RED");
				exit 1;
			}
		}

		if (not $with_dkms_install and $name =~ /-dkms/) {
			print_and_log("Skipping install of DKMS package ${name}-${version}.\n", (not $quiet));
			next;
		}

		my $deb_names = join " ", map {"'$_'"} @debs;
		print_and_log("Installing ${name}-${version}...\n", (not $quiet)) if(not $build_only);
		my @dpkg_extra_opts = ();
		if ($parent =~ /mlnx-ofed-kernel|libvma/) {
			$ENV{"PACKAGE_VERSION"} = "$version";
			push @dpkg_extra_opts, '--force-confmiss';
		}
		if (($distro =~ /ubuntu2[4-9]|debian1[3-9]/) && ($deb_name =~ /ibverbs/)) {
			push @dpkg_extra_opts, '--force-breaks';
		}
		ex_deb_install($name, "$DPKG -i @dpkg_extra_opts $DPKG_FLAGS $deb_names");

		my $ofa_src = "$builddir/ofed_src";
		my $ofa_dir = "$ofa_src/usr/src/ofa_kernel/$arch/${kernel}";

		if ($build_only and $name eq "mlnx-ofed-kernel-modules") {
			print_and_log("Extracting mlnx-ofed-kernel sources to $ofa_src ...\n", 1);
			system("mkdir -p $ofa_src >/dev/null 2>&1");
			my ($kdeb) = glob ("$DEBS/mlnx-ofed-kernel-modules[-_]${version}-*.kver.${kernel}_*.deb");
			if (not -e "$kdeb") {
				($kdeb) = glob ("$DEBS/mlnx-ofed-kernel-modules[-_]${version}-${kernel}_*.deb");
			}
			print_and_log("dpkg -x '$kdeb' $ofa_src >/dev/null \n", 1);
			system("dpkg -x '$kdeb' $ofa_src >/dev/null");
		}
		delete $ENV{"OFA_DIR"};
		if (-d $ofa_dir) {
			$ENV{"OFA_DIR"} = $ofa_dir;
		}

		# verify that kernel packages were successfuly installed
		if (not $build_only and exists $kernel_packages{"$name"}) {
			system("/sbin/depmod $kernel >/dev/null 2>&1");
			for my $object (@{$kernel_packages{"$name"}{"ko"}}) {
				my $file = `$MODINFO -k $kernel $object 2> /dev/null | grep filename | cut -d ":" -f 2 | sed -s 's/\\s//g'`;
				chomp $file;
				my $origin;
				if (-f $file) {
					$origin = `$DPKG -S '$file' 2> /dev/null | cut -d ":" -f 1`;
					chomp $origin;
				}
				if (not $file or $origin =~ /$kernel_escaped/) {
					print_and_log_colored("\nError: $name installation failed!", 1, "RED");
					if ($file) {
						print_and_log("Problem: $object: module file: $file, from package: $origin.\n", 1);
					} else {
						print_and_log("Problem: $object: module file not found.\n", 1);
					}
					addSetupInfo ("$ofedlogs/$name.debinstall.log");
					print_and_log_colored("See:\n\t$ofedlogs/$name.debinstall.log", 1, "RED");
					copy_make_log($name);
					print_and_log_colored("Removing newly installed packages...\n", 1, "RED");
					my $env_str = get_uninstall_env_str();
					ex "$env_str /usr/sbin/ofed_uninstall.sh --force";
					exit 1;
				}
			}
		}
	}

	foreach my $name (@selected_packages) {
		if ($packages_info{$name}{'do_not_release'}) {
			for my $deb (@{$packages_info{$name}{'built_debs'}}) {
				print_and_log_colored("Removing ${deb}...\n", 1, "RED");
				unlink $deb if (-e $deb);
			}
		}
	}
}

sub get_tarball_name_version
{
	my $tarname = shift @_;
	$tarname =~ s@.*/@@g;
	my $name = $tarname;
	$name =~ s/_.*//;
	$name =~ s/doca-//;

	my $version = $tarname;
	$version =~ s/^(doca-)?${name}_//;
	$version =~ s/(.orig).*//;

	return ($name, $version);
}

sub get_deb_name_version
{
	my $debname = shift @_;
	$debname =~ s@.*/@@g;
	my $name = $debname;
	$name =~ s/_.*//;
	my $version = $debname;
	$version =~ s/${name}_//;
	$version =~ s/_.*//;
	$version =~ s/-.*//;# remove release if available

	return ($name, $version);
}

sub get_deb_ver_inst
{
	my $ret;
	$ret = `$DPKG_QUERY -W -f='\${Version}\n' @_ | cut -d ':' -f 2 | uniq`;
	chomp $ret;
	return $ret;
}

sub set_existing_debs
{
	for my $deb (<"$DEBS/*.deb">) {
		my ($deb_name, $ver) = get_deb_name_version($deb);
		# skip unrelevnt debs
		if ($deb_name =~ /-modules/ and $deb !~ /-${kernel_escaped}_|\.kver\.${kernel_escaped}_/) {
			next;
		}

		$main_packages{$deb_name}{$ver}{'debpath'} = $deb;
		$packages_info{$deb_name}{$ver}{'deb_exist'} = 1;
		print_and_log("set_existing_debs: $deb_name $ver DEB exist\n", $verbose2);
	}
}

sub set_cfg
{
	my $tarball_full_path = shift @_;

	my ($name, $version) = get_tarball_name_version($tarball_full_path);

	$main_packages{$name}{$version}{'name'} = $name;
	$main_packages{$name}{$version}{'version'} = $version;
	$main_packages{$name}{$version}{'tarballpath'} = $tarball_full_path;

	print_and_log("set_cfg: " .
	"name: $name, " .
	"version: $main_packages{$name}{$version}{'version'}, " .
	"tarballpath: $main_packages{$name}{$version}{'tarballpath'}\n", $verbose3);
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

	my $scanned = 0;
	my $pname = $packages_info{$package}{'parent'};
	for my $ver (keys %{$main_packages{$pname}}) {
		$scanned = 1;

		# prevent loop
		if (not exists $packages_info{$package}{'entered_select_dependent'}) {
			$packages_info{$package}{'entered_select_dependent'}  = 1;
		} else {
			return 0 if (not $packages_info{$package}{'available'});
			my $parent = $packages_info{$package}{'parent'};
			return 0 if (not is_tarball_available($parent));
			return 1;
		}

		if ( not $packages_info{$package}{$ver}{'deb_exist'} ) {
			for my $req ( @{ $packages_info{$package}{'ofa_req_build'} } ) {
				next if not $req;
				# W/A for -p option and --user-space-only
				if ($req eq "$mlnX_ofed_kernel" and $print_available) {
					next;
				}
				print_and_log("resolve_dependencies: $package requires $req for deb build\n", $verbose2);
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
					$packages_info{$pname}{'available'} = 0;
					return 0;
				}
			}
		}

		for my $req ( @{ $packages_info{$package}{'ofa_req_inst'} } ) {
			next if not $req;
			last if $build_only;
			print_and_log("resolve_dependencies: $package requires $req for deb install\n", $verbose2);
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
				$packages_info{$pname}{'available'} = 0;
				return 0;
			}
		}

		if (not $packages_info{$package}{'selected'}) {
			return 0 if (not $packages_info{$package}{'available'});
			my $parent = $packages_info{$package}{'parent'};
			return 0 if (not is_tarball_available($parent));
			$packages_info{$package}{'selected'} = 1;
			push (@selected_packages, $package);
			print_and_log("select_dependent: Selected package $package\n", $verbose2);
			return 1;
		}
	}
	if ($scanned eq "0") {
		print_and_log("resolve_dependencies: $package does not exist. Skip dependencies check\n", $verbose2);
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
		print_and_log("select_dependent_module: $module requires $req for deb build\n", $verbose2);
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

	my $kernel_rpm = "$mlnX_ofed_kernel";
	my $pname = $packages_info{$kernel_rpm}{'parent'};
	for my $ver (keys %{$main_packages{$pname}}) {
		if ($packages_info{$kernel_rpm}{$ver}{'deb_exist'}) {
			for my $module (@selected_kernel_modules) {
				if (not is_module_in_deb($kernel_rpm, "$module")) {
					$packages_info{$kernel_rpm}{$ver}{'deb_exist'} = 0;
					$packages_info{'mlnx-ofed-kernel'}{$ver}{'deb_exist'} = 0;
					last;
				}
			}
			if ($with_memtrack) {
				if (not is_module_in_deb($kernel_rpm, "memtrack")) {
					$packages_info{$kernel_rpm}{$ver}{'deb_exist'} = 0;
					$packages_info{'mlnx-ofed-kernel'}{$ver}{'deb_exist'} = 0;
					last;
				}
			}
		}
	}
}

#
# set opensm service
#
sub set_opensm_service
{
	if ($enable_opensm or $install_option eq 'msm') {
		system("systemctl enable opensmd  > /dev/null 2>&1");
	} else {
		system("systemctl disable opensmd > /dev/null 2>&1");
	}
}

#
# set vma flags in /etc/modprobe.d/mlnx.conf
#
sub set_vma_flags
{
	return if ($user_space_only);
	my $mlnx_conf = "/etc/modprobe.d/mlnx.conf";
    if ($with_vma and -e "$mlnx_conf") {
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

    if (-f "/etc/infiniband/openib.conf") {
        my @lines;
        open(FD, "/etc/infiniband/openib.conf");
        while (<FD>) {
            push @lines, $_;
        }
        close (FD);
        # Do not start SDP
        # Do not start QIB to prevent http://bugs.openfabrics.org/bugzilla/show_bug.cgi?id=2262
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
}

sub print_selected
{
	print_and_log_colored("\nBelow is the list of ${PACKAGE} packages that you have chosen
	\r(some may have been added by the installer due to package dependencies):\n", 1, "GREEN");
	for my $package ( @selected_packages ) {
		print_and_log("$package\n", 1);
	}
	print_and_log("\n", 1);
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
	next if ($key =~ /mlx-steering-dump/ and $distro =~ /debian9/);
	# fix kernel packages names
	# DKMS enabled
	if ($with_dkms) {
		if ($key =~ /-modules/) {
			$key =~ s/-modules/-dkms/g;;
		}
	} else {
	# DKMS disabled
		if ($key =~ /-dkms/) {
			$key =~ s/-dkms/-modules/g;;
		}
	}

        if (exists $packages_info{$key}) {
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

	my $value = `tac '${kernel_sources}'/include/*/autoconf.h 2>/dev/null | grep -m1 ${VAR} 2>/dev/null | sed -ne 's/.*\\\([01]\\\)\$/\\1/gp' 2>/dev/null`;
	chomp $value;
	if ($value eq "") {
		$value = 0;
	}

	return $value;
}

sub set_availability
{
	if ($user_space_only) {
		$packages_info{"mlnx-ofed-kernel-utils"}{"mode"} = "user";
	}

	if ($arch =~ /arm|aarch/i) {
		$packages_info{'dapl'}{'available'} = 0;
		$packages_info{'libdapl2'}{'available'} = 0;
		$packages_info{'dapl2-utils'}{'available'} = 0;
		$packages_info{'libdapl-dev'}{'available'} = 0;
	}

	if ($kernel =~ /fbk/ or $arch =~ /arm|aarch/) {
		$kernel_modules_info{'sdp'}{'available'} = 0;
	}

	if (is_installed_deb("cuda-cudart-dev-$cuda_version") and $distro =~ /ubuntu/) {
		$cuda_top_dir = "/usr/local/cuda-$cuda_dir_name";
		# So we don't diverge in the support matrix from the UBS, we won't build ucx-cuda.
		# We'll take whatever binaries the UBS builds for the UCX package.
		# $packages_info{'ucx-cuda'}{'available'} = 1;
	}

	if ( not ($with_vma or $with_xlio) or $arch !~ m/x86_64|ppc64|arm|aarch/) {
		for my $p (qw/
			libvma libvma-utils libvma-dev
			sockperf
			libxlio libxlio-utils libxlio-dev
		/) {
			$packages_info{$p}{'available'} = 0;
		}
	}
	if (($distro !~ /ubuntu2[0246].04/x) or ($arch !~ /x86_64|aarch64/)) {
		for my $p (qw/libxlio libxlio-utils libxlio-dev/) {
			$packages_info{$p}{'available'} = 0;
		}
	}

        if (($distro !~ /ubuntu2[024].04|ubuntu18.04/x) or ($arch !~ /x86_64|aarch64/)) {
                for my $p (qw/libvma libvma-utils libvma-dev/) {
                        $packages_info{$p}{'available'} = 0;
                }
        }

        if ($arch =~ /ppc/) {
                for my $package (qw/
                        ucx ucx-cuda ucx-xpmem
                        openmpi
                        /)
                {
                        $packages_info{$package}{'available'} = 0;
                }
        }

	if ( $arch =~ m/aarch64/ and $with_bluefield) {
		if ($distro =~ /debian10 | debian11 | ubuntu18.04 | ubuntu20.04/x) {
			$packages_info{'nvme-snap'}{'available'} = 1;
			$packages_info{'spdk'}{'available'} = 1;
			$packages_info{'spdk-dev'}{'available'} = 1;
			$packages_info{'mlxbf-bootctl'}{'available'} = 1;
		}
	}

	if ($is_bf) {
		# Avoid rshim installation on BlueField
		$packages_info{'rshim'}{'available'} = 0;
	}

	# turn on isert if we are on follow OS and arch
	if (not ($distro =~ /
			ubuntu16.04 | ubuntu17.10 | ubuntu1[89] | ubuntu2. |
			debian9 | debian1.
		/x and	$kernel =~ /^[5-9] | ^4\.[12][0-9]\. | ^4\.[4-9] | ^3\.1[3-9]/x
	)) {
		$kernel_modules_info{'isert'}{'available'} = 0;
		$packages_info{'isert-dkms'}{'available'} = 0;
		$packages_info{'isert-modules'}{'available'} = 0;
	}
	if ($packages_info{$isert}{'available'}) {
		my $CONFIG_ISCSI_TARGET = check_autofconf('CONFIG_ISCSI_TARGET');
		if ($CONFIG_ISCSI_TARGET ne "1") {
			$kernel_modules_info{'isert'}{'available'} = 0;
			$packages_info{$isert}{'available'} = 0;
		}
	}

	# disable iproute2 for unsupported OSs
	if ($distro =~ /ubuntu1[45] | debian8/x) {
			$packages_info{'mlnx-iproute2'}{'available'} = 0;
	}

	if (not $with_dkms) {
		# we use only knem-modules when not working with dkms
		$packages_info{'knem'}{'available'} = 0;
	}
	if ($cross_compiling) {
		$packages_info{'knem'}{'available'} = 0;
		$packages_info{'knem-dkms'}{'available'} = 0;
		$packages_info{'knem-modules'}{'available'} = 0;
	}

	if ((($arch eq 'x86_64') and ($kernel !~ /
		^4\.15\.0-  # Ubuntu 18.04
		| ^5\.4\.0- # Ubuntu 20.04
		| ^5\.13\.0- # Ubuntu 21.10
		| ^5\.13\b
		| ^5\.15\b
		| ^5\.17\b   # Latest rebase's base
		| ^5\.18\b
		| ^5\.19\b
		| ^6\.[0-8]\b
		| ^6\.12\b	# Latest mainline
		| ^7\.0\b
		/x) ) or
		(($arch eq 'aarch64') and ($distro !~ /ubuntu2[246]\.04/)) or
		($arch !~ /aarch64 | x86_64/x)
	) {
		$kernel_modules_info{'nfsrdma'}{'available'} = 0;
		$packages_info{"$mlnx_nfsrdma"}{'available'} = 0;
	}

	if ($kernel !~ /^4\.[8-9] | ^4\.[12][0-9] | ^[5-9]/x) {
		$packages_info{"$mlnx_nvme"}{'available'} = 0;
	}

	if (not $with_fwctl) {
		$packages_info{"$fwctl"}{'available'} = 0;
	}

	if ((not $with_virtiofs) or ($kernel !~ /
		  ^6\.[6-9]\b
		| ^6.1[0-7]\b
		| ^7.0\b/x)) {
		$packages_info{"$virtiofs"}{'available'} = 0;
	}

	if (not ($distro =~ /ubuntu18.04|ubuntu2[0245].04/x and $arch =~ /x86_64|aarch64/x)) {
		$packages_info{"dpcp"}{'available'} = 0;
	}

	my $CONFIG_NET_UDP_TUNNEL = check_autofconf('CONFIG_NET_UDP_TUNNEL');
	if ($kernel !~ /^[5-9]|^4\.[8-9]|^4\.1[0-9]\./ or $CONFIG_NET_UDP_TUNNEL ne "1") {
		$packages_info{"$mlnx_rdma_rxe"}{'available'} = 0;
	}

	if ($with_mlxdevm) {
		if (
			(KV($kernel) < KV(4.15))
		) {
			print_and_log("mlxdevm requested but is not supported by kernel\n", $verbose);
			$with_mlxdevm = 0;
		}
	}
	$kernel_modules_info{'mlxdevm'}{'available'} = $with_mlxdevm;

	# turn off srp and iser if we are not on follow OS and arch
	if (not($distro =~ /
			ubuntu14.04 |
			ubuntu16.04 | ubuntu17.10 | ubuntu1[89] | ubuntu2. |
			debian8\.[7-9] | debian8\.1. | debian9 | debian1. | velinux
		/x and	$kernel =~ /^[4-9] | ^3\.1[6-9] | ^3.13.0-/x
	) ) {
		$kernel_modules_info{'srp'}{'available'} = 0;
		$packages_info{'srp'}{'available'} = 0;
		$packages_info{'srp-modules'}{'available'} = 0;
		$packages_info{'srp-dkms'}{'available'} = 0;
		$kernel_modules_info{'iser'}{'available'} = 0;
		$packages_info{'iser'}{'available'} = 0;
	}
	if ($packages_info{$iser}{'available'}) {
		my $CONFIG_ISCSI_TCP = check_autofconf('CONFIG_ISCSI_TCP');
		if ($CONFIG_ISCSI_TCP ne "1") {
			$kernel_modules_info{'iser'}{'available'} = 0;
			$packages_info{$iser}{'available'} = 0;
		}
	}

	if ($arch =~ /ppc64/) {
                $kernel_modules_info{'srp'}{'available'} = 0;
                $packages_info{'srp'}{'available'} = 0;
                $packages_info{'srp-modules'}{'available'} = 0;
                $packages_info{'srp-dkms'}{'available'} = 0;
	}

	if (not ($kernel =~ /^([4-9]|3\.1[0-9])/)) {
		$kernel_modules_info{'mlx5_fpga_tools'}{'available'} = 0;
	}

	# See https://redmine.mellanox.com/issues/1929856
	if ($distro eq 'ubuntu19.10') {
		block_package("ar-mgr");
	}

    ##############
    # handle without/disable flags
    if (keys %disabled_packages) {
        # build deps list
        for my $pkg (keys %packages_info) {
	    next if $build_only;
            for my $req ( @{ $packages_info{$pkg}{'ofa_req_inst'}} , @{ $packages_info{$pkg}{'ofa_req_build'}} ) {
		last if $build_only;
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

	# keep this at the end of the function.
	add_enabled_pkgs_by_user();
}

sub set_mlnx_tune
{
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
}

sub find_misconfigured_packages() {
	my @package_problems = ();
	open(DPKG, "dpkg-query -W -f '\${Status}::\${Package} \${Version}\n' |");
	while (<DPKG>) {
		chomp;
		my ($status_str, $package) = split('::');
		my ($required, $error, $status) = split(/ /, $status_str);
		next if ($status =~ /installed | config-files/x);
		push @package_problems, ("$package:\t$status_str\n");
	}
	close(DPKG);
	return @package_problems;
}

sub addSetupInfo
{
	my $log = shift @_;

	print "Collecting debug info...\n" if (not $quiet);

	if (not open (LOG, ">> $log")) {
		print "-E- Can't open $log for appending!\n";
		return;
	}
	my @package_problems = find_misconfigured_packages();

	print LOG "\n\n\n---------------- START OF DEBUG INFO -------------------\n";
	print LOG "Install command: $CMD\n";

	print LOG "\nVars dump:\n";
	print LOG "- ofedlogs: $ofedlogs\n";
	print LOG "- distro: $distro\n";
	print LOG "- arch: $arch\n";
	print LOG "- kernel: $kernel\n";
	print LOG "- config: $config\n";

	print LOG "\nSetup info:\n";
	print LOG "\n- uname -r: " . `uname -r 2>&1`;
	print LOG "\n- uname -m: " . `uname -m 2>&1`;
	print LOG "\n- lsb_release -a: " . `lsb_release -a 2>&1`;
	print LOG "\n- cat /etc/issue: " . `cat /etc/issue 2>&1`;
	print LOG "\n- cat /proc/version: " . `cat /proc/version 2>&1`;
	print LOG "\n- gcc --version: " . `gcc --version 2>&1`;
	print LOG "\n- lspci -n | grep 15b3: " . `lspci -n 2>&1 | grep 15b3`;
	if (@package_problems) {
		print LOG "\n- Potentiall broken packages:";
		print LOG "\n- ". join("\n- ", @package_problems);
	}
	print LOG "\n- dpkg --list: " . `dpkg --list 2>&1`;

	print LOG "---------------- END OF DEBUG INFO -------------------\n";
	close (LOG);
}

sub ex1
{
    my $cmd = shift @_;

    system("$cmd 2>&1");
    my $res = $? >> 8;
    my $sig = $? & 127;
    if ($sig or $res) {
        print_and_log_colored("Command execution failed: $cmd", 1, "RED");
        exit 1;
    }
}

sub uninstall
{
	return 0 if (not $uninstall);

	print_and_log("Removing old packages\n", 1);
	my $ofed_uninstall = "/do/not/run/ofed_uninstall.sh";
	chomp $ofed_uninstall;
	if (-f "$ofed_uninstall") {
		my $env_str = get_uninstall_env_str();
		print_and_log("Uninstalling the previous version of $PACKAGE\n", (not $quiet));
		if ($force) {
				system("yes | $env_str ofed_uninstall.sh --force >> $ofedlogs/ofed_uninstall.log 2>&1");
		} else {
				system("yes | $env_str ofed_uninstall.sh >> $ofedlogs/ofed_uninstall.log 2>&1");
		}
		my $res = $? >> 8;
		my $sig = $? & 127;
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
	my @list_to_remove;
	foreach (@remove_debs){
		next if ($_ =~ /^xen|ovsvf-config|opensmtpd/);
		foreach (get_all_matching_installed_debs($_)) {
			next if ($_ =~ /^xen|ovsvf-config|opensmtpd/);
			next if (not $components_filter->($_));
			if (not $selected_for_uninstall{$_}) {
				my $package = strip_package_arch($_);
				push (@list_to_remove, $_);
				$selected_for_uninstall{$package} = 1;
				if (not (exists $packages_info{$package} or $package =~ /mlnx-ofed-/)) {
					$non_ofed_for_uninstall{$package} = 1;
				}

				get_requires($_);
			}
		}
	}

	if (not $force and keys %non_ofed_for_uninstall) {
		print_and_log("\nError: One or more packages depends on MLNX_OFED.\nThose packages should be removed before uninstalling MLNX_OFED:\n\n", 1);
		print_and_log(join(" ", (keys %non_ofed_for_uninstall)) . "\n\n", 1);
		print_and_log("To force uninstallation use '--force' flag.\n", 1);
		exit $NONOFEDRPMS;
	}

	# verify that dpkg DB is ok
	print_and_log("Running: dpkg --configure -a --force-all --force-confdef --force-confold\n", $verbose2);
	system("dpkg --configure -a --force-all --force-confdef --force-confold >> '$glog' 2>&1");
	print_and_log("Running: apt-get install -f $apt_extra_params\n", $verbose2);
	system("apt-get install -f -y $apt_extra_params >> '$glog' 2>&1");

	my @list_to_remove_all = grep {not is_immuned($_)}
		(@list_to_remove, @dependant_packages_to_uninstall);
	ex "apt-get remove -y @list_to_remove_all" if (scalar(@list_to_remove_all));
	foreach (@list_to_remove_all){
		if (is_configured_deb($_)) {
			if (not /^opensm/) {
				ex "apt-get remove --purge -y $_";
			} else {
				system("apt-get remove --purge -y $_");
			}
		}
	}
	system ("/bin/rm -rf /usr/src/mlnx-ofed-kernel* > /dev/null 2>&1");
}

########
# MAIN #
########
sub main
{
	if (!$install_option) {
		$install_option = 'all';
	}

	if ($config_net_given) {
		if (not -e $config_net) {
			print_and_log_colored("Error: network config_file '$config_net' does not exist!", 1, "RED");
			exit 1;
		}

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

	set_availability();
	for my $tarball ( <$CWD/SOURCES/*> ) {
		set_cfg ($tarball);
	}

	my $num_selected = select_packages();
	set_existing_debs();
	resolve_dependencies();

	if (not $num_selected) {
		print_and_log_colored("$num_selected packages selected. Exiting...", 1, "RED");
		exit 1;
	}

	if ($print_available) {
		open(CONFIG, ">$config") || die "Can't open $config: $!";
		flock CONFIG, $LOCK_EXCLUSIVE;
		print "\nOFED packages: ";
		for my $package ( @selected_packages ) {
			my $parent = $packages_info{$package}{'parent'};
			next if (not $packages_info{$package}{'available'} or not is_tarball_available($parent));
			print("$package available: $packages_info{$package}{'available'}\n") if ($verbose2);
			if ($package =~ /$mlnX_ofed_kernel/) {
				print "\nKernel modules: ";
				for my $module ( @selected_kernel_modules ) {
					next if (not $kernel_modules_info{$module}{'available'});
					print $module . ' ';
					print CONFIG "$module=y\n";
				}
				print "\nRPMs: ";
			}
			print $package . ' ';
			print CONFIG "$package=y\n";
		}
		flock CONFIG, $UNLOCK;
		close(CONFIG);
		print GREEN "\nCreated $config", RESET "\n";
		exit $SUCCESS;
	}

	warn("Logs dir: $ofedlogs\n");
	warn("General log file: $glog\n");

	if (not $quiet and not $check_deps_only) {
		print_selected();
	}

	# install required packages
	check_linux_dependencies();

	print "This program will install the $PACKAGE package on your machine.\n"
	    . "Note that all other Mellanox, OEM, OFED, RDMA or Distribution IB packages will be removed.\n"
	    . "Those packages are removed due to conflicts with $PACKAGE, do not reinstall them.\n\n" if (not $quiet);

	uninstall();

	# install new packages chosen by the user
	install_selected();

	if ($build_only) {
		print_and_log_colored("Build passed successfully", (not $quiet), "GREEN");
		return 0;
	}

	if ($do_copy_udev) {
		copy_udev_rules($verbose2);
	}

	if (is_module_in_deb("mlnx-ofed-kernel", "ipoib")) {
		ipoib_config();
	}

	# set vma flags in /etc/modprobe.d/mlnx.conf in case the user chosen to enable vma
	set_vma_flags();

	set_mlnx_tune();

	# set opensm service
	set_opensm_service();

	if ($umad_dev_rw or $umad_dev_na) {
		set_umad_permissions($umad_dev_na);
	}

        if ( not $quiet ) {
            check_pcie_link();
        }

	print_and_log_colored("Installation passed successfully", (not $quiet), "GREEN");
}

main();

exit $SUCCESS;

