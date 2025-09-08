package host

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"

	"github.com/Mellanox/doca-driver-build/entrypoint/internal/constants"
	cmdMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/utils/cmd/mocks"
	osMockPkg "github.com/Mellanox/doca-driver-build/entrypoint/internal/wrappers/mocks"
)

var _ = Describe("Host", func() {
	Context("GetOSType", func() {
		var (
			h       Interface
			cmdMock *cmdMockPkg.Interface
			osMock  *osMockPkg.OSWrapper
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			h = New(cmdMock, osMock)
		})

		It("should return ubuntu for Ubuntu systems", func() {
			ubuntuOSRelease := `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
VERSION="22.04.3 LTS (Jammy Jellyfish)"
VERSION_CODENAME=jammy
ID=ubuntu
ID_LIKE=debian
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"
BUG_REPORT_URL="https://bugs.launchpad.net/ubuntu/"
PRIVACY_POLICY_URL="https://www.ubuntu.com/legal/terms-and-policies/privacy-policy"
UBUNTU_CODENAME=jammy`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(ubuntuOSRelease), nil)

			osType, err := h.GetOSType(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(osType).To(Equal(constants.OSTypeUbuntu))
		})

		It("should return sles for SLES systems", func() {
			slesOSRelease := `NAME="SLES"
VERSION="15-SP5"
VERSION_ID="15.5"
PRETTY_NAME="SUSE Linux Enterprise Server 15 SP5"
ID=constants.OSTypeSLES
ID_LIKE="suse"
ANSI_COLOR="0;32"
CPE_NAME="cpe:/o:suse:sles:15:sp5"
DOCUMENTATION_URL="https://www.suse.com/documentation/sles-15/"
LOGO="distributor-logo-SLES"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(slesOSRelease), nil)

			osType, err := h.GetOSType(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(osType).To(Equal(constants.OSTypeSLES))
		})

		It("should return redhat for RHEL systems", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
VERSION="9.2 (Plow)"
ID="rhel"
ID_LIKE="fedora"
VERSION_ID="9.2"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux 9.2 (Plow)"
ANSI_COLOR="0;31"
LOGO="fedora-logo-icon"
CPE_NAME="cpe:/o:redhat:enterprise_linux:9::baseos"
HOME_URL="https://www.redhat.com/"
DOCUMENTATION_URL="https://access.redhat.com/documentation/red_hat_enterprise_linux/9/"
SUPPORT_URL="https://access.redhat.com/support"
BUG_REPORT_URL="https://bugzilla.redhat.com/"
REDHAT_BUGZILLA_PRODUCT="Red Hat Enterprise Linux 9"
REDHAT_BUGZILLA_PRODUCT_VERSION=9.2
REDHAT_SUPPORT_PRODUCT="Red Hat Enterprise Linux"
REDHAT_SUPPORT_PRODUCT_VERSION="9.2"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil)

			osType, err := h.GetOSType(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(osType).To(Equal(constants.OSTypeRedHat))
		})

		It("should return redhat for CentOS systems", func() {
			centosOSRelease := `NAME="CentOS Stream"
VERSION="9"
ID="centos"
ID_LIKE="rhel fedora"
VERSION_ID="9"
PLATFORM_ID="platform:el9"
PRETTY_NAME="CentOS Stream 9"
ANSI_COLOR="0;31"
LOGO="fedora-logo-icon"
CPE_NAME="cpe:/o:centos:centos:9"
HOME_URL="https://www.centos.org/"
DOCUMENTATION_URL="https://docs.centos.org/"
SUPPORT_URL="https://www.centos.org/support/"
BUG_REPORT_URL="https://bugs.centos.org/"
REDHAT_SUPPORT_PRODUCT="CentOS Stream"
REDHAT_SUPPORT_PRODUCT_VERSION="9"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(centosOSRelease), nil)

			osType, err := h.GetOSType(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(osType).To(Equal(constants.OSTypeRedHat))
		})

		It("should handle case insensitive detection", func() {
			ubuntuOSRelease := `PRETTY_NAME="UBUNTU 22.04.3 LTS"
NAME="UBUNTU"
ID=UBUNTU`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(ubuntuOSRelease), nil)

			osType, err := h.GetOSType(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(osType).To(Equal(constants.OSTypeUbuntu))
		})

		It("should return error when /etc/os-release cannot be read", func() {
			osMock.EXPECT().ReadFile("/etc/os-release").Return(nil, assert.AnError)

			osType, err := h.GetOSType(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(osType).To(Equal(""))
		})
	})

	Context("GetRedHatVersionInfo", func() {
		var (
			h       Interface
			cmdMock *cmdMockPkg.Interface
			osMock  *osMockPkg.OSWrapper
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			h = New(cmdMock, osMock)
		})

		It("should return error when called on non-RedHat system", func() {
			ubuntuOSRelease := `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
ID=ubuntu`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(ubuntuOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("GetRedHatVersionInfo should only be called for RedHat-based distributions"))
			Expect(versionInfo).To(BeNil())
		})

		It("should parse RHEL 9.2 version correctly", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
VERSION="9.2 (Plow)"
ID="rhel"
ID_LIKE="fedora"
VERSION_ID="9.2"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux 9.2 (Plow)"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhelOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionInfo.MajorVersion).To(Equal(9))
			Expect(versionInfo.FullVersion).To(Equal("9.2"))
			Expect(versionInfo.OpenShiftVersion).To(Equal(""))
			Expect(versionInfo.OpenShiftVersion).To(Equal(""))
		})

		It("should parse RHEL 8.4 version correctly", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
VERSION="8.4 (Ootpa)"
ID="rhel"
ID_LIKE="fedora"
VERSION_ID="8.4"
PLATFORM_ID="platform:el8"
PRETTY_NAME="Red Hat Enterprise Linux 8.4 (Ootpa)"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhelOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionInfo.MajorVersion).To(Equal(8))
			Expect(versionInfo.FullVersion).To(Equal("8.4"))
			Expect(versionInfo.OpenShiftVersion).To(Equal(""))
			Expect(versionInfo.OpenShiftVersion).To(Equal(""))
		})

		It("should parse RHCOS with OpenShift version correctly", func() {
			rhcosOSRelease := `NAME="Red Hat Enterprise Linux CoreOS"
VERSION="412.86.202312151147-0"
ID="rhcos"
ID_LIKE="rhel fedora"
VERSION_ID="4.12"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux CoreOS 412.86.202312151147-0 (Plow)"
OPENSHIFT_VERSION="4.12"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhcosOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhcosOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionInfo.MajorVersion).To(Equal(4))
			Expect(versionInfo.FullVersion).To(Equal("4.12"))
			Expect(versionInfo.OpenShiftVersion).To(Equal("4.12"))
			Expect(versionInfo.OpenShiftVersion).ToNot(Equal(""))
		})

		It("should handle RHCOS without explicit OpenShift version", func() {
			rhcosOSRelease := `NAME="Red Hat Enterprise Linux CoreOS"
VERSION="412.86.202312151147-0"
ID="rhcos"
ID_LIKE="rhel fedora"
VERSION_ID="4.9"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux CoreOS 412.86.202312151147-0 (Plow)"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhcosOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhcosOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionInfo.MajorVersion).To(Equal(4))
			Expect(versionInfo.FullVersion).To(Equal("4.9"))
			Expect(versionInfo.OpenShiftVersion).To(Equal("4.9")) // default from shell script
			Expect(versionInfo.OpenShiftVersion).ToNot(Equal(""))
		})

		It("should handle RHCOS with ID=rhel but OPENSHIFT_VERSION present", func() {
			rhcosOSRelease := `NAME="Red Hat Enterprise Linux CoreOS"
VERSION="9.6.20250530-0 (Plow)"
ID="rhel"
ID_LIKE="fedora"
VERSION_ID="9.6"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux CoreOS 9.6.20250530-0 (Plow)"
VARIANT=CoreOS
VARIANT_ID=coreos
OPENSHIFT_VERSION="4.19"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhcosOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhcosOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionInfo.MajorVersion).To(Equal(9))
			Expect(versionInfo.FullVersion).To(Equal("9.6"))
			Expect(versionInfo.OpenShiftVersion).To(Equal("4.19"))
			Expect(versionInfo.OpenShiftVersion).ToNot(Equal(""))
		})

		It("should handle RHCOS with ID=rhcos and both OPENSHIFT_VERSION and RHEL_VERSION present", func() {
			rhcosOSRelease := `NAME="Red Hat Enterprise Linux CoreOS"
ID="rhcos"
ID_LIKE="rhel fedora"
VERSION="418.94.202506251005-0"
VERSION_ID="4.18"
VARIANT="CoreOS"
VARIANT_ID=coreos
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux CoreOS 418.94.202506251005-0"
ANSI_COLOR="0;31"
CPE_NAME="cpe:/o:redhat:enterprise_linux:9::baseos::coreos"
HOME_URL="https://www.redhat.com/"
DOCUMENTATION_URL="https://docs.okd.io/latest/welcome/index.html"
BUG_REPORT_URL="https://access.redhat.com/labs/rhir/"
REDHAT_BUGZILLA_PRODUCT="OpenShift Container Platform"
REDHAT_BUGZILLA_PRODUCT_VERSION="4.18"
REDHAT_SUPPORT_PRODUCT="OpenShift Container Platform"
REDHAT_SUPPORT_PRODUCT_VERSION="4.18"
OPENSHIFT_VERSION="4.18"
RHEL_VERSION=9.4
OSTREE_VERSION="418.94.202506251005-0"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhcosOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhcosOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionInfo.MajorVersion).To(Equal(4))
			Expect(versionInfo.FullVersion).To(Equal("4.18"))
			Expect(versionInfo.OpenShiftVersion).To(Equal("4.18"))
			Expect(versionInfo.OpenShiftVersion).ToNot(Equal(""))
		})

		It("should use RHEL_VERSION when present", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
VERSION="9.2 (Plow)"
ID="rhel"
VERSION_ID="9.2"
RHEL_VERSION="9.2.1"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhelOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionInfo.MajorVersion).To(Equal(9))
			Expect(versionInfo.FullVersion).To(Equal("9.2.1")) // RHEL_VERSION takes precedence
			Expect(versionInfo.OpenShiftVersion).To(Equal(""))
		})

		It("should use default version when VERSION_ID is missing", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
ID="rhel"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhelOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).ToNot(HaveOccurred())
			Expect(versionInfo.MajorVersion).To(Equal(8))
			Expect(versionInfo.FullVersion).To(Equal("8.4")) // default from shell script
			Expect(versionInfo.OpenShiftVersion).To(Equal(""))
		})

		It("should return error when /host/etc/os-release cannot be read", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
ID="rhel"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return(nil, assert.AnError)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read /host/etc/os-release"))
			Expect(versionInfo).To(BeNil())
		})

		It("should return error when major version cannot be parsed", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
ID="rhel"
VERSION_ID="invalid-version"`

			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil)
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhelOSRelease), nil)

			versionInfo, err := h.GetRedHatVersionInfo(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse major version"))
			Expect(versionInfo).To(BeNil())
		})
	})

	Context("Caching behavior", func() {
		var (
			h       Interface
			cmdMock *cmdMockPkg.Interface
			osMock  *osMockPkg.OSWrapper
		)

		BeforeEach(func() {
			cmdMock = cmdMockPkg.NewInterface(GinkgoT())
			osMock = osMockPkg.NewOSWrapper(GinkgoT())
			h = New(cmdMock, osMock)
		})

		It("should cache OS type and only read /etc/os-release once", func() {
			ubuntuOSRelease := `PRETTY_NAME="Ubuntu 22.04.3 LTS"
NAME="Ubuntu"
ID=ubuntu`

			// Expect only one call to ReadFile for /etc/os-release
			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(ubuntuOSRelease), nil).Once()

			// Call GetOSType multiple times
			osType1, err1 := h.GetOSType(context.Background())
			osType2, err2 := h.GetOSType(context.Background())
			osType3, err3 := h.GetOSType(context.Background())

			// All calls should return the same result
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())
			Expect(err3).ToNot(HaveOccurred())
			Expect(osType1).To(Equal(constants.OSTypeUbuntu))
			Expect(osType2).To(Equal(constants.OSTypeUbuntu))
			Expect(osType3).To(Equal(constants.OSTypeUbuntu))
		})

		It("should cache RedHat version info and only read files once", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
VERSION="9.2 (Plow)"
ID="rhel"
VERSION_ID="9.2"`

			// Expect only one call to each ReadFile
			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil).Once()
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return([]byte(rhelOSRelease), nil).Once()

			// Call GetRedHatVersionInfo multiple times
			versionInfo1, err1 := h.GetRedHatVersionInfo(context.Background())
			versionInfo2, err2 := h.GetRedHatVersionInfo(context.Background())
			versionInfo3, err3 := h.GetRedHatVersionInfo(context.Background())

			// All calls should return the same result
			Expect(err1).ToNot(HaveOccurred())
			Expect(err2).ToNot(HaveOccurred())
			Expect(err3).ToNot(HaveOccurred())
			Expect(versionInfo1.MajorVersion).To(Equal(9))
			Expect(versionInfo2.MajorVersion).To(Equal(9))
			Expect(versionInfo3.MajorVersion).To(Equal(9))
			Expect(versionInfo1.FullVersion).To(Equal("9.2"))
			Expect(versionInfo2.FullVersion).To(Equal("9.2"))
			Expect(versionInfo3.FullVersion).To(Equal("9.2"))
		})

		It("should cache errors and not retry on subsequent calls", func() {
			// Expect only one call to ReadFile that returns an error
			osMock.EXPECT().ReadFile("/etc/os-release").Return(nil, assert.AnError).Once()

			// Call GetOSType multiple times
			osType1, err1 := h.GetOSType(context.Background())
			osType2, err2 := h.GetOSType(context.Background())

			// Both calls should return the same error
			Expect(err1).To(HaveOccurred())
			Expect(err2).To(HaveOccurred())
			Expect(err1).To(Equal(err2))
			Expect(osType1).To(Equal(""))
			Expect(osType2).To(Equal(""))
		})

		It("should cache RedHat version errors and not retry on subsequent calls", func() {
			rhelOSRelease := `NAME="Red Hat Enterprise Linux"
ID="rhel"`

			// Expect only one call to each ReadFile
			osMock.EXPECT().ReadFile("/etc/os-release").Return([]byte(rhelOSRelease), nil).Once()
			osMock.EXPECT().ReadFile("/host/etc/os-release").Return(nil, assert.AnError).Once()

			// Call GetRedHatVersionInfo multiple times
			versionInfo1, err1 := h.GetRedHatVersionInfo(context.Background())
			versionInfo2, err2 := h.GetRedHatVersionInfo(context.Background())

			// Both calls should return the same error
			Expect(err1).To(HaveOccurred())
			Expect(err2).To(HaveOccurred())
			Expect(err1).To(Equal(err2))
			Expect(versionInfo1).To(BeNil())
			Expect(versionInfo2).To(BeNil())
		})
	})
})
