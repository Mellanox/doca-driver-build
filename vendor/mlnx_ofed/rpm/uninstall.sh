#!/usr/bin/perl
# vim: syntax=perl:ts=4:sw=4:sts=4:et
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
#
# Description: MLNX_OFED package uninstall script

use strict;
use warnings;
use Term::ANSIColor qw(:constants);
use File::Path;

my $PREREQUISIT = "172";
my $ERROR = "1";
my $NONOFEDRPMS = "174";

$ENV{"LANG"} = "C";

if ($<) {
    print RED "Only root can run $0", RESET "\n";
    exit $PREREQUISIT;
}

my $ofed_info = `which ofed_info 2> /dev/null`;
if (not $ofed_info) {
    print "No OFED installation detected. Exiting ...\n";
    exit $ERROR;
}

my @packages_to_uninstall = ();
my @dependant_packages_to_uninstall = ();
my %selected_for_uninstall = ();
my %non_ofed_for_uninstall = ();
my $unload_modules = 0;
my $force = 0;
my $verbose = 0;
my $quiet = 0;
my $dry_run = 0;
my $PACKAGE = `ofed_info -s | sed -e 's/://'`;
chomp $PACKAGE;
my $ofedlogs = "/tmp/$PACKAGE.$$.logs";
my $prefix = '/usr';
my $info = '/etc/infiniband/info';
my $rpm_flags = '';
my $keep_mft = 0;
my $is_mlnx_en = 0;
my $mlnx_en_pkgs = "";
my $mlnx_en_only_pkgs = "mlnx.*en|mstflint|ofed-scripts|mlnx-tools|mlnx-fw-updater|^rdma\$";
my $mlnx_en_rdma_pkgs = "$mlnx_en_only_pkgs|ofa_kernel|libibverbs|libmlx|librdmacm|libvma|liblxio|sockperf|rdma-core|rdma-core-devel|libibumad|infiniband-diags";
my @kernel_modules = qw/srp iser isert ofa_kernel knem ummunotify mlnx-sdp mlnx-rds mlnx-nfsrdma mlnx-nvme fwctl mlnx-rdma-rxe xpmem/;
my @components = qw/kernel user/;
my $components_filter;
my $should_remove_kernel;
my $components_str = 'all';
my %cli_only = ();

if ($PACKAGE =~ /mlnx-en/i) {
    $is_mlnx_en = 1;
    $keep_mft = 1;
}

sub usage
{
   print GREEN;
   print "\n Usage: $0 [--unload-modules] [-v|--verbose] [-q|--quiet] [--dry-run]\n";

   print "\n           --unload-modules     Run /etc/init.d/openibd stop before uninstall";
   print "\n           --keep-mft           Don't remove MFT package";
   print "\n           --only-kernel        Only remove kernel packages (this script included)";
   print "\n           --only-user          Only remove user-space packages (this script included)";
   print "\n           --force              Force uninstallation and remove packages that depends on MLNX_OFED";
   print "\n           -v|--verbose         Increase verbosity level";
   print "\n           --dry-run            Print the list of packages to be uninstalled without actually uninstalling them";
   print "\n           -q                   Set quiet - no messages will be printed";
   print RESET "\n\n";
}

sub getch
{
    my $c;
    $c=getc(STDIN);

    return $c;
}

sub log_and_exit
{
    my $rc = shift @_;
    if ($rc) {
        print RED "See logs under $ofedlogs", RESET "\n";
    }

    exit $rc;
}

# Find in file $name line containing str1 and replace it with str2
# If str2 is empty the line with str1 will be removed
sub find_and_replace
{
    my $name = shift @_;
    my $str1 = shift @_;
    my $str2 = shift @_;

    my @lines;
    open(FD, "$name");
    while (<FD>) {
        push @lines, $_;
    }
    close (FD);

    open(FD, ">$name");
    foreach my $line (@lines) {
        chomp $line;
        if ($line =~ /$str1/) {
            print FD "$str2\n" if ($str2);
        } else {
            print FD "$line\n";
        }
    }
    close (FD);
}

sub get_components_filter() {
    my $kernel_package_regex =
        '^kmod- | -kmp- | -modules$ | ^iser | ^srp$ | ^mlnx-nfsrdma$ | ^mlnx-nvme$ | fwctl | ^kernel-mft$';
    my $common_package_regex =
        '^mlnx-tools$ | ^ofed-scripts$ | ^mlnx-ofa_kernel | xpmem | ^knem$';
    my $has_kernel = grep {/^kernel/} @components;
    my $has_user = grep {/^user$/} @components;
    $should_remove_kernel = $has_kernel;
    if ($has_user and $has_kernel) {
        return sub {1};
    } elsif ($has_kernel) {
        $components_str = 'kernel';
        return sub {$_[0] =~ /$kernel_package_regex | $common_package_regex/x};
    } elsif ($has_user) {
        $components_str = 'user-space';
        return sub {$_[0] !~ /$kernel_package_regex/x};
    } else {
        print RED "Error: no kernel or user components found. Nothing to uninstall", RESET "\n";
        log_and_exit 3;
    }
}

sub is_installed
{
    my $res = 0;
    my $name = shift @_;

    system("rpm -q $name > /dev/null 2>&1");
    $res = $? >> 8;

    return not $res;
}

sub mark_for_uninstall
{
    my $package = shift @_;

    if (not $selected_for_uninstall{$package}) {
        push (@dependant_packages_to_uninstall, "$package");
        my $pname = $package;
        $pname =~ s@-[0-9].*@@g;
        $pname =~ s@-dev.*@@g;
        $selected_for_uninstall{$package} = 1;
        if ( `ofed_info 2>/dev/null | grep -i $pname 2>/dev/null` eq "" and $pname !~ /ofed-scripts|mlnx-ofed-|mlnx-fw-updater|neohost|infiniband-diags|doca-/x) {
            $non_ofed_for_uninstall{$package} = 1;
        }
    }
}

my %check_uninstall = ();
sub get_requires
{
    my $package = shift @_;

    # Strip RPM version
    $package = `rpm -q --queryformat "[%{NAME}]" $package`;
    chomp $package;

    if ($check_uninstall{$package}) {
        return; # already checked here
    }
    $check_uninstall{$package} = 1;

    if ($package eq "rdma") {
        # don't remove packages that needs rdma package
        return;
    }

    my @what_requires = `rpm -q --whatrequires $package 2>&1 | grep -v "no package requires" 2> /dev/null`;

    for my $pack_req (@what_requires) {
        chomp $pack_req;
        print "get_requires: $package is required by $pack_req\n" if ($verbose);
        get_requires($pack_req);
        mark_for_uninstall($pack_req);
    }
}

# Try to remove a package, if failed, retry with adding --noscripts flag
# Use it as W/A for known possible problematic packages
sub remove_od_noscripts
{
    my $pname = shift;

    my $cmd = "rpm -e --allmatches --nodeps $pname";
    print "\n$cmd\n" if (not $quiet);
    open (LOG, "+>$ofedlogs/ofed_uninstall.log");
    print LOG "$cmd\n";
    close LOG;
    if (not $dry_run) {
        system("$cmd >> $ofedlogs/ofed_uninstall.log 2>&1");
        my $res = $? >> 8;
        my $sig = $? & 127;
        if ($sig or $res) {
            $cmd = "rpm -e --allmatches --nodeps --noscripts $pname";
            print "\n$cmd\n" if (not $quiet);
            system("$cmd >> $ofedlogs/ofed_uninstall.log 2>&1");
            $res = $? >> 8;
            $sig = $? & 127;
            if ($sig or $res) {
                print RED "Failed to uninstall $pname", RESET "\n";
                addSetupInfo ("$ofedlogs/ofed_uninstall.log");
                print RED "See $ofedlogs/ofed_uninstall.log", RESET "\n";
                exit $ERROR;
            }
        }
    }
}

sub do_uninstall
{
    my $res = 0;
    my $sig = 0;
    my $cnt = 0;
    my @installed_rpms = `ofed_info | grep -A999 '^-' 2> /dev/null | grep -v '^-'`;
    my @other_ofed_rpms = `rpm -qa --queryformat "[%{NAME}]\n" 2> /dev/null | grep -wE "rdma|ofed|openib|mft|kernel-ib|rds|ib-bonding|infiniband|mlnx-en|mlnx_en|mlnx-fw-updater|neohost-backend|neohost-sdk|mlnx-dpdk|nvme-snap|spdk|rshim|mlxbf-bootctl" | grep -vEw "glusterfs|kernel-module|^xen|ovsvf-config|opensmtpd"`;
    my $cmd = "rpm -e --allmatches --nodeps $rpm_flags";

    my $keep_packages_regex = 'openvswitch|spdk';
    if (exists $ENV{'MLNX_OFED_KEEP_PACKAGES_REGEX'}) {
        $keep_packages_regex = $ENV{'MLNX_OFED_KEEP_PACKAGES_REGEX'};
    }
    @installed_rpms = grep !/$keep_packages_regex/, @installed_rpms;
    @other_ofed_rpms = grep !/$keep_packages_regex/, @other_ofed_rpms;
    for my $package (@other_ofed_rpms) {
        chomp $package;
        next if ($keep_mft and $package =~ /mft/);
        next if ($is_mlnx_en and $package !~ /$mlnx_en_pkgs/);
        my $pname = $package;
        $pname =~ s@-[0-9].*@@g;
        $pname =~ s@-dev.*@@g;
        if ( `ofed_info 2>/dev/null | grep -i $pname 2>/dev/null` eq "" and $pname !~ /ofed-scripts|mlnx-ofed-|mlnx-fw-updater|neohost|infiniband-diags|doca-/x ) {
            $non_ofed_for_uninstall{$package} = 1;
        }
    }

    for my $package (@installed_rpms, @other_ofed_rpms) {
        chomp $package;
        next if ($keep_mft and $package =~ /mft/);
        next if ($is_mlnx_en and $package !~ /$mlnx_en_pkgs/);
        next if ($package =~ /mpi-selector|glusterfs|^kernel-module|^xen|ovsvf-config|opensmtpd/);
        if (is_installed($package)) {
            push (@packages_to_uninstall, $package);
            $selected_for_uninstall{$package} = 1;
        }
    }

    for my $package (@packages_to_uninstall) {
        get_requires($package);
    }

    if (not $force and keys %non_ofed_for_uninstall) {
        print "\nError: One or more packages depends on MLNX_OFED.\nThose packages should be removed before uninstalling MLNX_OFED:\n\n";
        print join(" ", (keys %non_ofed_for_uninstall)) . "\n\n";
        print "To force uninstallation use '--force' flag.\n";
        exit $NONOFEDRPMS;
    }

    for my $package (@packages_to_uninstall, @dependant_packages_to_uninstall) {
        next if ($package =~ /pcp-pmda-infiniband/);
        if ("$package" eq "ofed" and is_installed("ofed")) {
            # W/A for SLES 10 SP4 in-box ofed RPM uninstall issue
            remove_od_noscripts("ofed");
            next;
        }
        if (is_installed("$package") and $components_filter->($package)) {
            $cmd .= " $package";
            $cnt ++;
        }
    }

    if ($cnt) {
        print "\n$cmd\n" if (not $quiet);
        open (LOG, "+>$ofedlogs/ofed_uninstall.log");
        print LOG "$cmd\n";
        close LOG;

        if (not $dry_run) {
            system("$cmd >> $ofedlogs/ofed_uninstall.log 2>&1");
            $res = $? >> 8;
            $sig = $? & 127;
            if ($sig or $res) {
                print RED "Failed to uninstall the previous installation", RESET "\n";
                print RED "See $ofedlogs/ofed_uninstall.log", RESET "\n";
                log_and_exit $ERROR;
            }
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

    my $DISTRO = `grep -iE "Red Hat.*7\." /etc/issue /etc/*release* 2>/dev/null`;
    chomp $DISTRO;
    if ($DISTRO eq "") {
        return;
    }

    # get list of installed packages
    for my $package (@kernel_modules, qw/kernel-mft/) {
        my $regEx = "*$package*";
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
    if ((not $dry_run) and $should_remove_kernel) {
        remove_weak_updates_symlinks();
    }

    do_uninstall();
}


######### MAIN #########
if (exists $ENV{MLNX_OFED_COMPONENTS}) {
	@components = grep {/^(kernel[0-9a-zA-Z._-]*|user)$/}
        (split ' ', $ENV{MLNX_OFED_COMPONENTS});
}

while ( $#ARGV >= 0 ) {

   my $cmd_flag = shift(@ARGV);

    if ($cmd_flag eq "--unload-modules") {
        $unload_modules = 1;
    } elsif ($cmd_flag eq "--force") {
        $force = 1;
    } elsif ($cmd_flag eq "-v" or $cmd_flag eq "--verbose") {
        $verbose = 1;
    } elsif ($cmd_flag eq "-q" or $cmd_flag eq "--quiet") {
        $quiet = 1;
    } elsif ($cmd_flag eq "--dry-run") {
        $dry_run = 1;
    } elsif ($cmd_flag eq "--keep-mft") {
        $keep_mft = 1;
    } elsif ($cmd_flag eq "--only-kernel") {
        @components = qw/kernel/;
        $cli_only{'kernel'} = 1;
    } elsif ($cmd_flag eq "--only-user") {
        @components = qw/user/;
        $cli_only{'user'} = 1;
    } elsif ($cmd_flag eq "-h" or $cmd_flag eq "--help") {
        usage();
        exit 0;
    }
}
$components_filter = get_components_filter();
if (2 <= keys %cli_only) {
    print RED "Command-line options --only-kernel and --only-user are incompatible. Aborting.", RESET "\n";
    exit $ERROR;
}

if (not $force) {
    print "\nThis program will uninstall $components_str $PACKAGE packages on your machine.\n\n";
    print "Do you want to continue?[y/N]:";
    my $ans = getch();
    print "\n";

    if ($ans !~ m/[yY]/) {
        exit $ERROR;
    }
}

if (is_installed("mlnx-ofa_kernel")) {
    $mlnx_en_pkgs = $mlnx_en_rdma_pkgs;
} else {
    $mlnx_en_pkgs = $mlnx_en_only_pkgs;
}

mkpath([$ofedlogs]);

if (-e $info) {
    open(INFO, "$info|") or die "Failed to run: $info. Error $!\n";;
    while(<INFO>) {
        if (/^prefix=/) {
            $prefix = (split '=', $_)[1];
            last;
        }
    }
    close(INFO);
} else {
    $prefix = $0;
    $prefix =~ s/(\/sbin).*//g;
}

if ($unload_modules) {
    print "Unloading kernel modules...\n" if (not $quiet);

    if (not $dry_run) {
        system("/etc/init.d/openibd stop >> $ofedlogs/openibd_stop.log 2>&1");
        my $res = $? >> 8;
        my $sig = $? & 127;
        if ($sig or $res) {
            print RED "Failed to unload kernel modules", RESET "\n";
            log_and_exit $ERROR;
        }
    }
}

uninstall();

exit 0 if ($dry_run);

if (-e "/etc/sysctl.conf") {
    system ("grep -q MLNX_nr_overcommit_hugepages /etc/sysctl.conf");
    my $res = $? >> 8;
    my $sig = $? & 127;
    if (not ($sig or $res)) {
        find_and_replace ("/etc/sysctl.conf", "nr_overcommit_hugepages", "");
    }
}

if (-e "/etc/modprobe.d/ipv6") {
   find_and_replace ("/etc/modprobe.d/ipv6", "# install ipv6 /bin/true", "install ipv6 /bin/true");
}

system("/sbin/modprobe -r knem > /dev/null 2>&1");
# RH
system("/bin/rm -f /etc/sysconfig/modules/knem.modules");
# SLES
system("sed -i -e 's/ knem//g' /etc/sysconfig/kernel 2>/dev/null");

print "Uninstall finished successfully\n" if (not $quiet);

# VIM: this is perl:
# vi:set syntax=perl:sts=4:sw=4:expandtab:
