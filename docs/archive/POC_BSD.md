# FreeBSD VM PoC - pf divert pass-through

## Purpose

Validate pf divert capture/reinject for LAN -> WAN HTTPS before adding split logic.

## Variables

- lan_if: LAN interface name (example vtnet1 or em1)
- wan_if: WAN interface name (example vtnet0 or em0)
- lan_net: LAN subnet CIDR (example 192.168.10.0/24)
- divert_port: divert port (example 10000)
- anchor_path: /etc/pf.anchors/gov-pass

## Interface selection

Pick one anchor template:
- WAN-scoped (default): docs/pf/gov-pass.anchor.wan.conf
- LAN-scoped: docs/pf/gov-pass.anchor.lan.conf
- All interfaces (no interface restriction): docs/pf/gov-pass.anchor.all.conf

pfSense (Proxmox, virtio) presets:
- LAN-scoped: docs/pf/gov-pass.anchor.pfsense.proxmox.lan.conf
- WAN-scoped: docs/pf/gov-pass.anchor.pfsense.proxmox.wan.conf
- All interfaces: docs/pf/gov-pass.anchor.pfsense.proxmox.all.conf

## FreeBSD VM checklist

1) VM topology: 2 NICs (LAN + WAN), pf enabled.
2) Identify interfaces:

```sh
ifconfig -l
netstat -rn
```

3) Copy the selected anchor template to the anchor path and edit variables.

4) Add these lines to /etc/pf.conf:

```pf
anchor "gov-pass"
load anchor "gov-pass" from "/etc/pf.anchors/gov-pass"
```

5) Validate and reload pf:

```sh
pfctl -vnf /etc/pf.conf
pfctl -f /etc/pf.conf
pfctl -s info
```

6) Run the PoC divert binary (recv + reinject pass-through).

7) Verify traffic:

```sh
tcpdump -ni <wan_if> tcp port 443
curl -sk https://example.com >/dev/null
```

8) If reinjection loops occur, confirm the bypass rule is first and the tag
   name matches in the anchor file.

## pfSense (Proxmox NAT) concrete setup

### Proxmox host (example NAT bridge)

Assumes:
- vmbr0: existing uplink bridge (internet)
- vmbr1: NAT bridge for pfSense WAN (example 172.16.100.0/24)
- vmbr2: isolated bridge for pfSense LAN (no host IP)

Example /etc/network/interfaces (adapt to your host):

```text
auto vmbr1
iface vmbr1 inet static
  address 172.16.100.1/24
  bridge-ports none
  bridge-stp off
  bridge-fd 0
  post-up iptables -t nat -A POSTROUTING -s 172.16.100.0/24 -o vmbr0 -j MASQUERADE
  post-up iptables -A FORWARD -i vmbr1 -o vmbr0 -j ACCEPT
  post-up iptables -A FORWARD -i vmbr0 -o vmbr1 -m state --state RELATED,ESTABLISHED -j ACCEPT
  post-down iptables -t nat -D POSTROUTING -s 172.16.100.0/24 -o vmbr0 -j MASQUERADE
  post-down iptables -D FORWARD -i vmbr1 -o vmbr0 -j ACCEPT
  post-down iptables -D FORWARD -i vmbr0 -o vmbr1 -m state --state RELATED,ESTABLISHED -j ACCEPT

auto vmbr2
iface vmbr2 inet manual
  bridge-ports none
  bridge-stp off
  bridge-fd 0
```

Enable IP forwarding on the host (persist via sysctl.conf as needed):

```sh
sysctl -w net.ipv4.ip_forward=1
```

If your host uses nftables, translate the NAT rules accordingly.

### pfSense VM

1) Create a pfSense VM with 2 virtio NICs:
   - net0 -> vmbr1 (WAN NAT)
   - net1 -> vmbr2 (LAN)
2) Assign interfaces:
   - WAN = vtnet0
   - LAN = vtnet1
3) Set LAN IP to 10.0.0.1/24 and enable DHCP (example range 10.0.0.100-10.0.0.200).
4) Confirm WAN receives an IP from vmbr1 (DHCP or static).

### LAN client VM

1) Attach a separate client VM to vmbr2.
2) Verify it gets a DHCP lease from pfSense and can reach the internet.

### pf divert PoC (manual, non-persistent)

1) Verify divert support:

```sh
sysctl net.inet.ip.divert
```

2) Copy the LAN-scoped preset to pfSense and edit if needed:

```sh
cp /path/to/docs/pf/gov-pass.anchor.pfsense.proxmox.lan.conf /etc/pf.anchors/gov-pass
```

3) Append the anchor to the current ruleset and load:

```sh
cp /tmp/rules.debug /tmp/rules.gov-pass
printf '\nanchor "gov-pass"\nload anchor "gov-pass" from "/etc/pf.anchors/gov-pass"\n' >> /tmp/rules.gov-pass
pfctl -vnf /tmp/rules.gov-pass
pfctl -f /tmp/rules.gov-pass
```

4) Confirm the anchor is active:

```sh
pfctl -a gov-pass -sr
```

5) Revert:

```sh
pfctl -f /tmp/rules.debug
```

This is a PoC workflow. For persistence, use a pfSense filter reload hook or a
shellcmd-style startup task once the behavior is validated.

## pfSense persistence options

All options below use the same apply script. Create it once on pfSense:

```sh
cat > /usr/local/bin/gov-pass-apply.sh <<'SH'
#!/bin/sh
set -e
ANCHOR="/etc/pf.anchors/gov-pass"
RULES_BASE="/tmp/rules.debug"
RULES_OUT="/tmp/rules.gov-pass"

if [ ! -f "$ANCHOR" ] || [ ! -f "$RULES_BASE" ]; then
  exit 0
fi

cp "$RULES_BASE" "$RULES_OUT"
printf '\nanchor "gov-pass"\nload anchor "gov-pass" from "%s"\n' "$ANCHOR" >> "$RULES_OUT"
pfctl -vnf "$RULES_OUT"
pfctl -f "$RULES_OUT"
SH
chmod +x /usr/local/bin/gov-pass-apply.sh
```

### Option 1: Shellcmd package (boot-time)

Use this if you only need the anchor re-applied on boot.

1) Install the `shellcmd` package.
2) Go to Services -> Shellcmd and add:
   - Command: `/usr/local/bin/gov-pass-apply.sh`
   - Type: `shellcmd` (use `earlyshellcmd` if you see timing issues)

### Option 2: Cron package (periodic re-apply)

Use this if you want the anchor to recover after filter reloads.

1) Install the `cron` package.
2) Create a job to run `/usr/local/bin/gov-pass-apply.sh` every 1 minute.
3) Validate that a filter reload is followed by re-application within the
   interval.

### Option 3: Filter reload hook (advanced)

This is the most robust approach, but it depends on pfSense internals and is
version-sensitive. If you already maintain a local pfSense package/overlay,
wire a filter reload hook to call `/usr/local/bin/gov-pass-apply.sh` after
`/tmp/rules.debug` is regenerated.

If you do not have a package/overlay workflow, use Option 2 (cron) as a safe
stopgap and only move to a hook once PoC behavior is stable.

Decision for this PoC: skip the hook and use Option 2 (cron).

## Notes

- This PoC is for a FreeBSD VM. pfSense/OPNsense manage pf.conf; do not edit
  /etc/pf.conf on those appliances.
- Offload settings may affect packet boundaries; disable TSO/LRO/CSUM during
  validation if needed.
- If WAN-scoped rules do not match because NAT runs before filtering, use the
  LAN-scoped anchor or drop the "from $lan_net" clause.
- For pfSense, verify interface names in the UI or with `ifconfig -l`. virtio
  typically uses vtnet0 (WAN) and vtnet1 (LAN), but this can vary.
- lan_net should be a network CIDR (example 10.0.0.0/24), not the LAN IP.
