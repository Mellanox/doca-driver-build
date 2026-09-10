#!/usr/bin/perl
#
# Copyright (c) 2015 Mellanox Technologies. All rights reserved.
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
use File::Compare;
use Cwd;
use Term::ANSIColor qw(:constants);
$ENV{"LANG"} = "C";

# Predeclare shared globals:
sub ex;
use vars qw($verbose $verbose2);
use vars qw($ofedlogs);

my $setpci = 'setpci';
my $lspci = 'lspci';

my $PCI_CAP_ID_EXP       = "0x10";     # PCI Express
my $PCI_EXP_LNKSTA       = "0x12";     # Link Status
my $PCI_EXP_LNKSTA_WIDTH = "0x03f0";   # Negotiated Link Width
my $PCI_EXP_LNKSTA_SPEED = "0x000f";   # Negotiated Link Speed
my $PCI_EXP_LNKCAP       = "0xc";      # Link Capabilities
my $PCI_EXP_LNKCAP_WIDTH = "0x003f0";  # Maximum Link Width
my $PCI_EXP_LNKCAP_SPEED = "0x0000f";  # Maximum Link Speed
my $PREREQUISIT = "172";

sub non_root_user() {
    return $<;
}

sub check_root_user() {
    if (non_root_user()) {
        print RED "Only root can run $0", RESET "\n";
        exit $PREREQUISIT;
    }
}

sub clean_path {
    my $file = shift @_;

    # remove trailing '/'
    $file =~ s/\/$//;

    # remove double '//'
    while ($file =~ /\/\//) {
        $file =~ s/\/\//\//g;
    }

    return $file;
}

sub hexSum
{
        my $a = shift @_;
        my $b = shift @_;

        $a = hex "$a";
        $b = hex "$b";

        my $val = sprintf("%x", $a + $b);
}

# type: can be one of: W, B, L (see setpci man pages)
sub pci_read
{
        my $dev = shift @_;
        my $offset = shift @_;
        my $type = shift @_;

        my $val = `$setpci -s $dev ${offset}.${type}`;
        chomp $val;
        my $res = $? >> 8;
        my $sig = $? & 127;
        if ($sig or $res or $res =~ /Unaligned/i) {
                $val = "Failed to read!";
        }
        return $val;
}

# get offset of PCI Express Capability structure
# (element in a linked list)
sub pci_find_pcie_cap_structure
{
        my $dev = shift @_;

        my $offset = pci_read($dev, '0x34', 'B');
        if ($offset =~ /ff/) {
                return -1;
        }
        while (hex($offset)) {
                my $id = pci_read($dev, ${offset}, 'B');
                if (hex "$id" == hex $PCI_CAP_ID_EXP) {
                        return $offset;
                }
                if ($id =~ /Fail|ff/) {
                        return -1;
                }
                $offset = hexSum($offset, 1);
                $offset = pci_read($dev, ${offset}, 'B');
        }

        return 0;
}

sub pci_get_lnksta
{
        my $dev = shift @_;
        my $cap_offset = shift @_;

        my $offset = hexSum($cap_offset, $PCI_EXP_LNKSTA);
        return pci_read($dev, $offset, 'W');
}

sub pci_get_lnkcap
{
        my $dev = shift @_;
        my $cap_offset = shift @_;

        my $offset = hexSum($cap_offset, $PCI_EXP_LNKCAP);
        return pci_read($dev, $offset, 'W');
}

sub pci_get_link_width
{
        my $dev = shift @_;
        my $cap_offset = shift @_;

        my $lnksta = pci_get_lnksta($dev, $cap_offset);
        return ((hex $lnksta) & (hex $PCI_EXP_LNKSTA_WIDTH )) >> 4;
}

sub pci_get_link_width_cap
{
        my $dev = shift @_;
        my $cap_offset = shift @_;

        my $lnkcap = pci_get_lnkcap($dev, $cap_offset);
        return ((hex $lnkcap) & (hex $PCI_EXP_LNKCAP_WIDTH )) >> 4;
}

sub pci_get_link_speed
{
        my $dev = shift @_;
        my $cap_offset = shift @_;

        my $lnksta = pci_get_lnksta($dev, $cap_offset);
        my $speed = ((hex $lnksta) & (hex $PCI_EXP_LNKSTA_SPEED ));
        # PCIe Gen1 = 2.5GT/s signal-rate per lane with 8/10 encoding    = 0.25GB/s data-rate per lane
        # PCIe Gen2 = 5  GT/s signal-rate per lane with 8/10 encoding    = 0.5 GB/s data-rate per lane
        # PCIe Gen3 = 8  GT/s signal-rate per lane with 128/130 encoding = 1   GB/s data-rate per lane
        # PCIe Gen4 = 16 GT/s signal-rate per lane with 128/130 encoding = 2   GB/s data-rate per lane
        if ( $speed eq "1" ) {
            return "2.5GT/s";
        } elsif ( $speed eq "2" ) {
            return "5GT/s";
        } elsif ( $speed eq "3" ) {
            return "8GT/s";
        } elsif ( $speed eq "4" ) {
            return "16GT/s";
        } else {
            return "Unknown";
        }
}

sub check_pcie_link
{
        for my $devid ( `$lspci -d 15b3: 2>/dev/null | cut -d" " -f"1"` ) {
            chomp $devid;

            my $hdr = pci_find_pcie_cap_structure($devid);
            if ("$hdr" eq "-1") {
                next;
            }
            my $link_width = pci_get_link_width($devid, $hdr);
            if ("$link_width" eq "0") {
                next;
            }
            my $link_width_cap = pci_get_link_width_cap($devid, $hdr);

            print "Device ($devid):\n";
            print "\t" . `$lspci -s $devid`;

            print "\tLink Width: x${link_width}";
            if ("$link_width" ne "$link_width_cap") {
                print YELLOW " ( WARNING - device supports x${link_width_cap} )";
                print RESET "";
            }
            print "\n";

            my $link_speed = pci_get_link_speed($devid, $hdr);
            print "\tPCI Link Speed: $link_speed\n";
            print "\n";
        }
}

sub unsupported_devices() {
	my @devices = ();
	open LSPCI, "$lspci -nn -d 15b3:|";
	while (<LSPCI>) {
		if (/15b3:(
			1002| 6368| 6732| 673c| 6740| 6750| 6764| # CTX2
			1003| 1004| 1007|	# ConnectX-3
			1013|			# ConnectX-4
			1023|			# ConnectX-8
			1025|			# ConnectX-9
			1011| 1012|		# Connect-IB
			a2dc|			# BlueField-3
			____
			)/x) {
			push @devices, $_;
		}
	}
	close(LSPCI);
	return @devices;
}

sub unsupported_devices_check() {
	my @devices = unsupported_devices();
	return unless (@devices);

	my $error = "System has one or more unsupported device, see below.\n";
	$error .= "ConnectX-8/BlueField-3 and newer devices are only supported in DOCA-host and not in MLNX_OFED / mlnx_en\n";
	$error .= "ConnectX-3 devices are only supported in MLNX_OFED / mlnx_en 4.9 and below.\n";
	$error .= "ConnectX-4 devices are only supported in MLNX_OFED / mlnx_en 24.10 and below.\n";
	$error .= "This device could become unavailable which might result in loss of connectivity.\n" ;
	$error .= "Use --skip-unsupported-devices-check to skip this check.\n" ;
	$error .= "Aborting.";
	print_and_log_colored($error, 1, "RED");
	foreach my $dev (@devices) {
		print_and_log_colored("* $dev", 1, "RED");
	}
	exit 1;
}

sub size_to_netmask($) {
	my $size = shift;
	my $bit = (2** (32 - $size)) - 1;
	my ($full_mask) = unpack("N", pack("C4", 255,255,255,255));
	my $netmask  = join('.', unpack("C4", pack("N", ($full_mask ^ $bit))));

	return $netmask;
}

sub KV($) {
	my $ver = shift @_;
	$ver =~ s/(\d+.\d+)\..*/$1/;
	my @nums = split /[.]/, $ver;
	my $ver_num = $nums[0] * 1000 + $nums[1];
	return $ver_num;
}

sub get_net_config($$$)
{
    my $interface = shift;
    my $ifcfg = shift;
    my $DISTRO = shift;

    my $cmd = "ip address show $interface";
    open(IP, "$cmd |") or die "Failed to run $cmd: $!";
    while (<IP>) {
        next unless (m/ inet /);
	my ($ig_inet, $inet, $ig_brd, $brd, $ignored) = split ' ';
	my ($inet_ip, $net_size) = split '/', $inet;
        $ifcfg->{$interface}{'IPADDR'} = $inet_ip;
        $ifcfg->{$interface}{'BROADCAST'} = $brd;
        $ifcfg->{$interface}{'NETMASK'} = size_to_netmask($net_size);
        if ($DISTRO =~ /RHEL6|RHEL7|POWERKVM|EULEROS2.0SP[235]|BCLINUX7|ALIOS/) {
            $ifcfg->{$interface}{'NM_CONTROLLED'} = "yes";
            $ifcfg->{$interface}{'TYPE'} = "InfiniBand";
        }
    }
    close(IP);
}

sub get_components_filter($$) {
	my $is_deb = shift;
	my $components = shift;

	my $kernel_package_regex =
		'^kmod- | -kmp- | -modules$ | ^iser | ^srp$ | ^mlnx-nfsrdma$ | ^mlnx-nvme$ | ^kernel-mft$';
	my $common_package_regex =
		'^mlnx-tools$ | ^ofed-scripts$ | ^mlnx-ofa_kernel | xpmem | ^knem$';
	if ($is_deb) {
		$kernel_package_regex = '-dkms$ | modules$';
		$common_package_regex =
			'^ofed_scripts$ | ^mlnx-tools$ | ^mlnx-ofed-kernel-utils$ | ^mlnx-en-utils$ | ^knem$';
	}

	my $has_kernel = grep {/^kernel/} @$components;
	my $has_user = grep {/^user$/} @$components;
	if ($has_user and $has_kernel) {
		return  sub {1};
	} elsif ($has_kernel) {
		return sub {$_[0] =~ /$kernel_package_regex | $common_package_regex/x};
	} elsif ($has_user) {
		return sub {$_[0] !~ /$kernel_package_regex/x};
	}
	print_and_log_colored("Error: no kernel or user components found. Nothing to uninstall", 1, "RED");
	exit 3;
}

sub parse_rpm_dist($$) {
	my $dist_rpm = shift;
	my $kernel = shift;

	my $DISTRO = "";
	my $rpm_distro = "";

        if ($dist_rpm =~ /system-release-2-17.amzn2/) {
                $DISTRO = "AL2";
                $rpm_distro = "amzn2";
	} elsif ($dist_rpm =~ /system-release-2023/) {
		$DISTRO = "AL23";
		$rpm_distro = "amzn23";
        } elsif ($dist_rpm =~ /openSUSE-release-12.1/) {
		$DISTRO = "openSUSE12.1";
		$rpm_distro = "opensuse12sp1";
	} elsif ($dist_rpm =~ /openSUSE-release-20151203-1.1/) {
		$DISTRO = "openSUSE_TUMBLEWEED";
		$rpm_distro = "opensuse_tumbleweed";
	} elsif ($dist_rpm =~ /openSUSE-release-15.3/) {
                $DISTRO = "openSUSE15.3";
                $rpm_distro = "opensuse15sp3";
	} elsif ($dist_rpm =~ /(?:sles|SLES|SLE_HPC|SLES_SAP)-release-(1[\d])(|\.([\d]))-/) {
		# example: dist_rpm: "sles-release-15.1-39.1"
		#					"sles-release-12-1.377"
		my $sp = $2? $3 : "0";
		$DISTRO = "SLES$1";			   # "SLES15"
		$rpm_distro = "sles${1}sp${sp}";  # "sles15sp1"
	} elsif ($dist_rpm =~ /sles-release-12\.4|SLES.*release-12\.4/) {
		$DISTRO = "SLES12";
		$rpm_distro = "sles12sp4";
	} elsif ($dist_rpm =~ /sles-release-12\.3|SLES.*release-12\.3/) {
		$DISTRO = "SLES12";
		$rpm_distro = "sles12sp3";
	} elsif ($dist_rpm =~ /sles-release-12\.2|SLES.*release-12\.2/) {
		$DISTRO = "SLES12";
		$rpm_distro = "sles12sp2";
	} elsif ($dist_rpm =~ /sles-release-12\.1|SLES.*release-12\.1/) {
		$DISTRO = "SLES12";
		$rpm_distro = "sles12sp1";
	} elsif ($dist_rpm =~ /sles-release-12|SLES.*release-12/) {
		$DISTRO = "SLES12";
		$rpm_distro = "sles12sp0";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.0|centos-release-6-0/) {
		$DISTRO = "RHEL6.0";
		$rpm_distro = "rhel6u0";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.1[.-]|sl-release-6.1[.-]|centos-release-6-1[.-]/) {
		$DISTRO = "RHEL6.1";
		$rpm_distro = "rhel6u1";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.2|sl-release-6.2|centos-release-6-2/) {
		$DISTRO = "RHEL6.2";
		$rpm_distro = "rhel6u2";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.3|sl-release-6.3|centos-release-6-3/) {
		$DISTRO = "RHEL6.3";
		$rpm_distro = "rhel6u3";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.4|sl-release-6.4|centos-release-6-4/) {
		$DISTRO = "RHEL6.4";
		$rpm_distro = "rhel6u4";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.5|sl-release-6.5|centos-release-6-5/) {
		$DISTRO = "RHEL6.5";
		$rpm_distro = "rhel6u5";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.6|sl-release-6.6|centos-release-6-6/) {
		$DISTRO = "RHEL6.6";
		$rpm_distro = "rhel6u6";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.7|sl-release-6.7|centos-release-6-7/) {
		$DISTRO = "RHEL6.7";
		$rpm_distro = "rhel6u7";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.8|sl-release-6.8|centos-release-6-8/) {
		$DISTRO = "RHEL6.8";
		$rpm_distro = "rhel6u8";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.9|sl-release-6.9|centos-release-6-9/) {
		$DISTRO = "RHEL6.9";
		$rpm_distro = "rhel6u9";
	} elsif ($dist_rpm =~ /redhat-release-.*-6.10|sl-release-6.10|centos-release-6-10/) {
		$DISTRO = "RHEL6.10";
		$rpm_distro = "rhel6u10";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.0|sl-release-7.0|centos-release-7-0/) {
		$DISTRO = "RHEL7.0";
		$rpm_distro = "rhel7u0";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.1[.-]|sl-release-7.1[.-]|centos-release-7-1[.-]/) {
		$DISTRO = "RHEL7.1";
		$rpm_distro = "rhel7u1";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.2|sl-release-7.2|centos-release-7-2/) {
		$DISTRO = "RHEL7.2";
		$rpm_distro = "rhel7u2";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.3|sl-release-7.3|centos-release-7-3/) {
		$DISTRO = "RHEL7.3";
		$rpm_distro = "rhel7u3";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.4.*el7a|sl-release-7.4.*el7a|centos-release-7-4.*el7.centos.a/) {
		$DISTRO = "RHEL7.4ALTERNATE";
		$rpm_distro = "rhel7u4alternate";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.5.*el7a|sl-release-7.5.*el7a|centos-release-7-5.*el7.centos.a/) {
		$DISTRO = "RHEL7.5ALTERNATE";
		$rpm_distro = "rhel7u5alternate";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.6.*el7a|sl-release-7.6.*el7a|centos-release-7-6.*el7.centos.a/) {
		$DISTRO = "RHEL7.6ALTERNATE";
		$rpm_distro = "rhel7u6alternate";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.4|sl-release-7.4|centos-release-7-4/) {
		$DISTRO = "RHEL7.4";
		$rpm_distro = "rhel7u4";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.5|sl-release-7.5|centos-release-7-5/) {
		$DISTRO = "RHEL7.5";
		$rpm_distro = "rhel7u5";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.6|sl-release-7.6|centos-release-7-6/) {
		$DISTRO = "RHEL7.6";
		$rpm_distro = "rhel7u6";
		if ($kernel =~ /^4\.14.*el7/) {
			$DISTRO = "RHEL7.6ALTERNATE";
			$rpm_distro = "rhel7u6alternate";
		}
	} elsif ($dist_rpm =~ /redhat-release-.*-7.7|sl-release-7.7|centos-release-7-7/) {
		$DISTRO = "RHEL7.7";
		$rpm_distro = "rhel7u7";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.8|sl-release-7.8|centos-release-7-8/) {
		$DISTRO = "RHEL7.8";
		$rpm_distro = "rhel7u8";
	} elsif ($dist_rpm =~ /redhat-release-.*-7.9|sl-release-7.9|centos-release-7-9/) {
		$DISTRO = "RHEL7.9";
		$rpm_distro = "rhel7u9";
	} elsif ($dist_rpm =~ /(redhat|centos|centos-linux|almalinux|rocky|centos-stream)-release-([89]|10)\.(\d+)-/) {
		my ($major, $minor) = ($2, $3);
		if (
			(($major eq "8") and ($minor <= 10)) ||
			(($major eq "9") and ($minor <= 7)) ||
			(($major eq "10") and ($minor <= 2))
		) {
			$DISTRO = "RHEL$2.$3";
			$rpm_distro = "rhel$2u$3";
		}
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-1\./) {
		$DISTRO = "OL6.1";
		$rpm_distro = "ol6u1";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-2/) {
		$DISTRO = "OL6.2";
		$rpm_distro = "ol6u2";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-3/) {
		$DISTRO = "OL6.3";
		$rpm_distro = "ol6u3";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-4/) {
		$DISTRO = "OL6.4";
		$rpm_distro = "ol6u4";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-5/) {
		$DISTRO = "OL6.5";
		$rpm_distro = "ol6u5";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-6/) {
		$DISTRO = "OL6.6";
		$rpm_distro = "ol6u6";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-7/) {
		$DISTRO = "OL6.7";
		$rpm_distro = "ol6u7";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-8/) {
		$DISTRO = "OL6.8";
		$rpm_distro = "ol6u8";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-9/) {
		$DISTRO = "OL6.9";
		$rpm_distro = "ol6u9";
	} elsif ($dist_rpm =~ /oraclelinux-release-6.*-10/) {
		$DISTRO = "OL6.10";
		$rpm_distro = "ol6u10";
	} elsif ($dist_rpm =~ /oraclelinux-release-([7-9])\.(\d+)-/) {
		my ($major, $minor) = ($1, $2);
		if (
			(($major eq "7") and ($minor <= 9)) ||
			(($major eq "8") and ($minor <= 10)) ||
			(($major eq "9") and ($minor <= 6))
		) {
			$DISTRO = "OL$1.$2";
			$rpm_distro = "ol$1u$2";
		}
	} elsif ($dist_rpm =~ /redhat-release-.*-5.8|centos-release-5-8|enterprise-release-5-8/) {
		$DISTRO = "RHEL5.8";
		$rpm_distro = "rhel5u8";
	} elsif ($dist_rpm =~ /bclinux-release-(7)-([3-7])/) {
		$DISTRO = "BCLINUX$1.$2";
		$rpm_distro = "bclinux${1}u${2}";
	} elsif ($dist_rpm =~ /bclinux-release-(8).([12])/) {
		$DISTRO = "BCLINUX$1.$2";
		$rpm_distro = "bclinux${1}u${2}";
	} elsif ($dist_rpm =~ /bclinux-release-(21)\.(10)/) {
		$DISTRO = "BCLINUX$1.$2SP2";
		$rpm_distro = "bclinux${1}u${2}sp2";
	} elsif ($dist_rpm =~ /bclinux-release-22\.10/) {
		$DISTRO = "BCLINUX22.10";
		 $rpm_distro = "bclinux22u10.oe2203sp2.bclinux";
	} elsif ($dist_rpm =~ /centos-release-5-10.el5.centos/) {
		$DISTRO = "XenServer6.5";
		$rpm_distro = "xenserver6u5";
	} elsif ($dist_rpm =~ /xenserver-release-7.2.0/) {
		$DISTRO = "XenServer7.2";
		$rpm_distro = "xenserver7u2";
	} elsif ($dist_rpm =~ /xenserver-release-7.1.[02]/) {
		$DISTRO = "XenServer7.1";
		$rpm_distro = "xenserver7u1";
	} elsif ($dist_rpm =~ /xenserver-release-7.0.0/) {
		$DISTRO = "XenServer7.0";
		$rpm_distro = "xenserver7u0";
	} elsif ($dist_rpm =~ /xenserver-release-8.2.1/) {
		$DISTRO = "XenServer8.2";
		$rpm_distro = "xenserver8u2";
	} elsif ($dist_rpm =~ /redhat-release-.*-5.7|centos-release-5-7|enterprise-release-5-7/) {
		if ($kernel =~ /xs/) {
			$DISTRO = "XenServer6.2";
			$rpm_distro = "xenserver6u2";
		} else {
			$DISTRO = "RHEL5.7";
			$rpm_distro = "rhel5u7";
		}
	} elsif ($dist_rpm =~ /redhat-release-.*-5.6|centos-release-5-6|enterprise-release-5-6/) {
		$DISTRO = "RHEL5.6";
		$rpm_distro = "rhel5u6";
	} elsif ($dist_rpm =~ /redhat-release-.*-5.5|centos-release-5-5|enterprise-release-5-5/) {
		system("grep -wq XenServer /etc/issue > /dev/null 2>&1");
		my $res = $? >> 8;
		my $sig = $? & 127;
		if ($sig or $res) {
			$DISTRO = "RHEL5.5";
			$rpm_distro = "rhel5u5";
		} else {
			$DISTRO = "XenServer5.6";
			$rpm_distro = "xenserver5u6";
		}
	} elsif ($dist_rpm =~ /redhat-release-.*-5.4|centos-release-5-4/) {
		$DISTRO = "RHEL5.4";
		$rpm_distro = "rhel5u4";
	} elsif ($dist_rpm =~ /redhat-release-.*-5.3|centos-release-5-3/) {
		$DISTRO = "RHEL5.3";
		$rpm_distro = "rhel5u3";
	} elsif ($dist_rpm =~ /redhat-release-.*-5.2|centos-release-5-2/) {
		$DISTRO = "RHEL5.2";
		$rpm_distro = "rhel5u2";
	} elsif ($dist_rpm =~ /redhat-release-4AS-9/) {
		$DISTRO = "RHEL4.8";
		$rpm_distro = "rhel4u8";
	} elsif ($dist_rpm =~ /redhat-release-4AS-8/) {
		$DISTRO = "RHEL4.7";
		$rpm_distro = "rhel4u7";
	} elsif ($dist_rpm =~ /fedora-release(|-common)-(\d+)/ and
			($2 >= 12) and ($2 <= 44)) {
		$DISTRO = "FC$2";
		$rpm_distro = "fc$2";
	} elsif ($dist_rpm =~ /base-files-3.0/) {
		system("grep -q \"Bluenix 1.0\" /etc/issue > /dev/null 2>&1");
		my $res = $? >> 8;
		my $sig = $? & 127;
		if ($sig or $res) {
			$DISTRO = "WINDRIVER6.0";
			$rpm_distro = "windriver6u0";
		} else {
			$DISTRO = "BLUENIX1.0";
			$rpm_distro = "bluenix1u0";
		}
	} elsif ($dist_rpm =~ /ibm_powerkvm-release-3.1.0/) {
		$DISTRO = "POWERKVM3.1.0";
		$rpm_distro = "powerkvm3u1u0";
	} elsif ($dist_rpm =~ /ibm_powerkvm-release-3.1.1/) {
		$DISTRO = "POWERKVM3.1.1";
		$rpm_distro = "powerkvm3u1u1";
	} elsif ($dist_rpm =~ /euleros-release-2.0SP([23589]|1[0-3])/) {
		my $sp = $1;
		$DISTRO = "EULEROS2.0SP${sp}";
		$rpm_distro = "euleros2u0sp${sp}";
	} elsif ($dist_rpm =~ /openEuler-release-(2[02]\.03)LTS_SP(1|3)/) {
		my $dist_num = $1;
		my $sp = $2;
		$DISTRO = "OPENEULER${dist_num}SP${sp}";
		$dist_num =~ y/./u/;
		$rpm_distro = "openeuler${dist_num}sp${sp}";
	} elsif ($dist_rpm =~ /openEuler-release-(2[024]\.03)/) {
		my $dist_num = $1;
		$DISTRO = "OPENEULER$dist_num";
		$dist_num =~ y/./u/;
		$rpm_distro = "openeuler$dist_num";
	} elsif ($dist_rpm =~ /alios-release-server-7.2/) {
		$DISTRO = "ALIOS7.2";
		$rpm_distro = "alios7u2";
	} elsif ($dist_rpm =~ /kylin-release-11/) {
		$DISTRO = "KYLIN11";
		$rpm_distro = "kylin11";
	} elsif ($dist_rpm =~ /UnionTech-release-(10[267]0)/) {
		$DISTRO = "UOS20.$1";
		$rpm_distro = "uos20.$1";
	} elsif ($dist_rpm =~ /UnionTech-release-20-(10[67]0a)/) {
		$DISTRO = "UOS20.$1";
		$rpm_distro = "uos20.$1";
	} elsif ($dist_rpm =~ /photon-release-3.0-/) {
		$DISTRO = "PHOTON3";
		$rpm_distro = "photon3";
	} elsif ($dist_rpm =~ /mariner-release-1.0/) {
		$DISTRO = "MARINER1.0";
		$rpm_distro = "mariner1.0";
	} elsif ($dist_rpm =~ /mariner-release-2.0/) {
		$DISTRO = "MARINER2.0";
		$rpm_distro = "mariner2u0";
	} elsif ($dist_rpm =~ /azurelinux-release-3.0/) {
		$DISTRO = "AZURELINUX3.0";
		$rpm_distro = "azurelinux3u0";
	} elsif ($dist_rpm =~ /anolis-release-8.([46])/) {
		$DISTRO = "ANOLIS8.$1";
		$rpm_distro = "anolis8u$1";
	} elsif ($dist_rpm =~ /ctyunos-release-2.22.06/) {
		$DISTRO = "CTYUNOS22.06";
		$rpm_distro = "ctyunos22.06";
	} elsif ($dist_rpm =~ /ctyunos-release-23.01/) {
		$DISTRO = "CTYUNOS23.01";
		$rpm_distro = "ctyunos23.01";
	} elsif ($dist_rpm =~ /ctyunos-release-4_25.07/) {
		$DISTRO = "CTYUNOS25.07";
		$rpm_distro = "ctyunos25.07";
	} elsif ($dist_rpm =~ /tencentos-release-4.4/) {
		$DISTRO = "TENCENTOS4.4";
		$rpm_distro = "tencentos4u4";
	} elsif ($dist_rpm =~ /tencentos-release-3.3/) {
		$DISTRO = "TENCENTOS3.3";
		$rpm_distro = "tencentos3u3";
	} elsif ($dist_rpm =~ /tencentos-release/) {
		$DISTRO = "TENCENT";
		$rpm_distro = "tencent";
	} elsif ($dist_rpm =~ /alinux-release-3.2104/) {
		$DISTRO = "ALINUX3.12";
		$rpm_distro = "alinux3u12";
	} elsif ($dist_rpm =~ /alinux-release-3/) {
		$DISTRO = "ALINUX3.2";
		$rpm_distro = "alinux3u2";
	} elsif ($dist_rpm =~ /alinux-release-4/) {
		$DISTRO = "ALINUX4";
		$rpm_distro = "alinux4";
	}
	return ($rpm_distro, $DISTRO);
}

sub print_script_version() {
	print "1\n";
}

sub purge_failed_dkms_packages() {
	my $fh;
	my @failed = ();
	open $fh, "env COLUMNS=200 dpkg -l '*-dkms' 2>/dev/null|";
	while(<$fh>) {
		next unless /^.F/;
		my ($status, $package, $version, $rest) = split ' ';
		push @failed, $package;
	}
	close $fh;
	return unless (@failed);
	my $packages = join " ", @failed;
	print_and_log("Purging failed DKMS packages: dpkg --purge $packages\n", $verbose);
	ex "dpkg --purge $packages";
}

sub get_umad_dev_rule($) {
	my $others_ro = shift;
	my $mode = '"0666"';
	if ($others_ro) {
		$mode = '"0660"'
	}
	return 'KERNEL=="umad*", NAME="infiniband/%k", MODE='."$mode\n";
}

sub set_umad_permissions($)
{
	my $others_ro = shift;

	my $status = "noline";
	my $ib_udev_rules = "/etc/udev/rules.d/91-ib-permissions.rules";
	my @ib_udev_rules_lines = ();
	if (-r $ib_udev_rules) {
		open(IB_UDEV_RULES, $ib_udev_rules) or die "Can't open $ib_udev_rules: $!";
		@ib_udev_rules_lines = <IB_UDEV_RULES>;
		close(IB_UDEV_RULES);
	}
	foreach my $line (@ib_udev_rules_lines) {
		next unless ($line =~ /^KERNEL=="umad/);
		$status = "found";
		my $orig_rule = $line;
		# Changes array element:
		$line = get_umad_dev_rule($others_ro);
		if ($line ne $orig_rule) {
			$status = "changed";
		}
	}
	if ($status eq "noline") {
		push @ib_udev_rules_lines, get_umad_dev_rule($others_ro);
		$status = "changed";
	}

	if ($status eq "changed") {
		open(IB_UDEV_RULES, ">$ib_udev_rules") or die "Can't write to $ib_udev_rules: $!";
		foreach (@ib_udev_rules_lines) {
			print IB_UDEV_RULES;
		}
		close IB_UDEV_RULES;
	}
}

sub copy_file($$$) {
	my $src = shift;
	my $dst = shift;
	my $v = shift;
	my $rc = File::Copy::copy($src, $dst);
	if (not $rc) {
		print_and_log("Error: copy of $src to $dst failed: $!\n", 1);
		return $rc;
	}
	print_and_log("Copied $src to $dst.\n", $v);
	return 1;
}

# Copy example "legacy" udev rules (only 82-*) and the supporting
# scripts from the examples directory to /etc. Override any existing.
sub copy_udev_rules($) {
	my $v = shift;

	my @dirs = qw(
		/usr/share/doc/mlnx-ofa_kernel
		/usr/share/doc/mlnx-ofa_kernel-[1-9]*
		/usr/share/doc/packages/mlnx-ofa_kernel
		/usr/share/doc/mlnx-ofed-kernel-utils/examples
	);
	my $examples_dir = "";
	foreach (@dirs) {
		my @g = glob($_);
		next unless (@g);
		my $d = $g[0];
		if (-d $d) {
			$examples_dir = $d;
			last;
		}
	}
	if (not $examples_dir) {
		print_and_log("Error: missing examples / docs directory, cannot copy example udev rules.\n", 1);
		return 0;
	}
	my $udev_file = "$examples_dir/82-net-setup-link.rules";
	my $script = "$examples_dir/vf-net-link-name.sh";
	copy_file($script, "/etc/infiniband/", $v) or return 0;
	copy_file($udev_file, "/etc/udev/rules.d/", $v) or return 0;
	chmod 0755, "/etc/infiniband/vf-net-link-name.sh";
	return 1;
}

sub copy_make_log($) {
	my $name = shift;

	my $makelog = `grep "make.log" $ofedlogs/$name.debinstall.log 2>/dev/null`;
	if ($makelog =~ /.*\s(.*make.log)\s.*/) {
		$makelog = $1;
	}
	if (not -f $makelog) {
		my $path = `grep -Ei "/var/lib/dkms/.*build.*for more information" $ofedlogs/$name.debinstall.log 2>/dev/null`;
		if ($path =~ /.*(\/var\/lib\/dkms\/.*build).*/) {
			$makelog = "$1/$makelog";
		}
	}
	if (-f $makelog) {
		system("cp $makelog $ofedlogs/$name.make.log");
		print_and_log_colored("\t$ofedlogs/$name.make.log", 1, "RED");
	}
}

sub debian_version() {
	open(DEB_VER, "/etc/debian_version")
		or die("Failed to open /etc/debian_version: $!");
	my $version = <DEB_VER>;
	chomp $version;
	if ($version !~ /^[1-9]/) {
		$version = 0; # e.g.: Debian Testing / Unstable
	}
	return $version;
}

{
	# This scope is needed to allow os_release to keep state between
	# calls.
	# FIXME: once we remove support for ancient OSes (XenServer 6.5
	# and the likes, this should be replaced with 'state' and this
	# scope could be removed.

	# The state:
	my $initialized = 0;
	my %os_release = ();

	sub os_release($) {
		my $param = shift;

		if (not $initialized) {
			open(OS_RELEASE, "/etc/os-release")
				or die("Failed to open /etc/os-release: $!");
			while (<OS_RELEASE>) {
				my ($key, $val) = split(/[=]/, $_, 2);
				chomp $val;
				$val =~ s/^"(.*)"/$1/;
				$os_release{$key} = $val;
			}
			$initialized = 1;
			close(OS_RELEASE);
			# Debian does not provide minor version numbers here.
			# Get them elsewhere:
			if($os_release{"ID"} eq "debian") {
				$os_release{"VERSION_ID"} = debian_version();
			} elsif ($os_release{"ID"} eq "UOS") {
				my $uos_version = debian_version();
				if ($uos_version eq "10.10") {
					$os_release{"VERSION_ID"} = "20.1040";
				}
			}
		}
		if (exists $os_release{$param}) {
			return $os_release{$param};
		}
		return undef;
	}
}

#:# PackagesSubdirectory

1;
