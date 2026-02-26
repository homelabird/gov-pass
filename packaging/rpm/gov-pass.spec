%global gov_pass_default_version 0.1.3

Name:           gov-pass
Version:        %{?gov_pass_version}%{!?gov_pass_version:%{gov_pass_default_version}}
Release:        %{?gov_pass_release}%{!?gov_pass_release:1%{?dist}}
Summary:        Split-only TLS ClientHello splitter for outbound TCP/443

License:        GPL-3.0-only
URL:            https://github.com/homelabird/gov-pass
Source0:        https://github.com/homelabird/gov-pass/archive/refs/tags/v%{version}.tar.gz#/%{name}-%{version}.tar.gz
Source1:        gov-pass.service
Source2:        gov-pass.sysconfig

ExclusiveArch:  x86_64
BuildRequires:  golang >= 1.21
BuildRequires:  gcc
BuildRequires:  systemd-rpm-macros
Requires:       systemd
Requires:       ethtool
Requires:       iproute
Recommends:     nftables
Recommends:     iptables
%{?systemd_requires}

%description
gov-pass is a split-only TLS ClientHello splitter for outbound TCP/443 traffic.
On Linux, it uses NFQUEUE for packet interception and raw-socket
packet forwarding.

This package installs:
- splitter runtime binary
- NFQUEUE install/uninstall helper scripts
- systemd unit file
- /etc/sysconfig override file

%prep
%autosetup -n %{name}-%{version}

%build
export CGO_ENABLED=1
go build -buildmode=pie -o dist/splitter ./cmd/splitter

%check
go test ./internal/... ./cmd/splitter/...

%install
install -d %{buildroot}%{_libexecdir}/%{name}
install -m 0755 dist/splitter %{buildroot}%{_libexecdir}/%{name}/splitter
install -m 0755 scripts/linux/install_nfqueue.sh %{buildroot}%{_libexecdir}/%{name}/install_nfqueue.sh
install -m 0755 scripts/linux/uninstall_nfqueue.sh %{buildroot}%{_libexecdir}/%{name}/uninstall_nfqueue.sh

install -D -m 0644 %{SOURCE1} %{buildroot}%{_unitdir}/gov-pass.service
install -D -m 0644 %{SOURCE2} %{buildroot}%{_sysconfdir}/sysconfig/gov-pass

%post
%systemd_post gov-pass.service

%preun
%systemd_preun gov-pass.service

%postun
%systemd_postun_with_restart gov-pass.service

%files
%license LICENSE
%doc README.md docs/PACKAGING.md docs/THIRD_PARTY_NOTICES.md
%config(noreplace) %{_sysconfdir}/sysconfig/gov-pass
%{_unitdir}/gov-pass.service
%{_libexecdir}/%{name}/splitter
%{_libexecdir}/%{name}/install_nfqueue.sh
%{_libexecdir}/%{name}/uninstall_nfqueue.sh

%changelog
* Thu Feb 26 2026 gov-pass maintainers <maintainers@example.com> - 0.1.3-1
- Add initial RPM packaging spec with systemd/sysconfig integration
