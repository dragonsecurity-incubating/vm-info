#!/usr/bin/env bash
set -euo pipefail

# vm-info.sh — Pretty VM summary for libvirt
# Usage:
#   ./vm-info.sh
#   ./vm-info.sh --wide
#   ./vm-info.sh --filter-cidr 10.244.0.0/16 [--filter-cidr A.B.C.D/M]
#   ./vm-info.sh --disks [--wide] [--filter-cidr ...]
#
# Requires: virsh. Optional: jq (for QGA JSON).

show_disks=0
wide_ips=0
declare -a filter_cidrs=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --disks) show_disks=1; shift ;;
    --wide)  wide_ips=1; shift ;;
    --filter-cidr) filter_cidrs+=("$2"); shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

has_jq=0
command -v jq >/dev/null 2>&1 && has_jq=1

# --- Helpers ---------------------------------------------------------------

cidr_contains() {
  # cidr_contains <CIDR> <IP>  -> 0 if in CIDR, 1 otherwise
  local cidr="$1" ip="$2"
  # Busybox/ash-friendly pure bash matcher for IPv4
  IFS=/ read -r net mask <<<"$cidr"
  IFS=. read -r n1 n2 n3 n4 <<<"$net"
  IFS=. read -r i1 i2 i3 i4 <<<"$ip"
  # sanity
  [[ -z "$n1" || -z "$i1" || -z "$mask" ]] && return 1
  local nb=$(( (n1<<24) + (n2<<16) + (n3<<8) + n4 ))
  local ib=$(( (i1<<24) + (i2<<16) + (i3<<8) + i4 ))
  local m=$(( 0xFFFFFFFF << (32-mask) & 0xFFFFFFFF ))
  [[ $((nb & m)) -eq $((ib & m)) ]]
}

ip_allowed() {
  # returns 0 if IP is allowed after filters, 1 if it should be hidden
  local ip="$1"
  # Exclude loopback by default
  [[ "$ip" == 127.* ]] && return 1
  for c in "${filter_cidrs[@]:-}"; do
    if cidr_contains "$c" "$ip"; then
      return 1
    fi
  done
  return 0
}

join_csv() { awk 'BEGIN{ORS=""; first=1} {if(NF){if(!first) printf(","); printf("%s",$0); first=0}} END{print ""}'; }
trim_commas() { sed -E 's/^[, ]+//; s/[, ]+$//; s/,+/,/g;'; }

get_dominfo_field () {
  virsh dominfo "$1" 2>/dev/null | awk -F: -v k="$2" '$1~k{gsub(/^[ \t]+/, "", $2); print $2}'
}

get_vcpus () {
  # Prefer vcpucount current; fallback to dominfo
  local vm="$1"
  if out=$(virsh vcpucount "$vm" --current 2>/dev/null); then
    echo "$out" | awk -F: '/current/ {gsub(/[^0-9]/,"",$2); print $2; exit}'
    return
  fi
  get_dominfo_field "$vm" "CPU(s)"
}

to_mib () {
  local num unit
  num=$(awk '{print $1}' <<<"$1")
  unit=$(awk '{print $2}' <<<"$1")
  case "${unit^^}" in
    KIB) awk -v n="$num" 'BEGIN{printf "%.0f", n/1024}' ;;
    MIB|"") awk -v n="$num" 'BEGIN{printf "%.0f", n}' ;;
    GIB) awk -v n="$num" 'BEGIN{printf "%.0f", n*1024}' ;;
    *) echo "$num" ;;
  esac
}

get_hostname () {
  local vm="$1"
  if hn=$(virsh domhostname "$vm" 2>/dev/null); then
    [[ -n "$hn" ]] && { echo "$hn"; return; }
  fi
  if [[ $has_jq -eq 1 ]]; then
    if out=$(virsh qemu-agent-command "$vm" '{"execute":"guest-get-host-name"}' --timeout 2 2>/dev/null); then
      jq -r '.return' <<<"$out" 2>/dev/null && return
    fi
    if out=$(virsh qemu-agent-command "$vm" '{"execute":"guest-info"}' --timeout 2 2>/dev/null); then
      jq -r '.return.hostname // empty' <<<"$out" 2>/dev/null && return
    fi
  fi
  echo "-"
}

get_ipv4s_all () {
  local vm="$1" ips out
  if [[ $has_jq -eq 1 ]]; then
    if out=$(virsh qemu-agent-command "$vm" '{"execute":"guest-network-get-interfaces"}' --timeout 2 2>/dev/null); then
      ips=$(jq -r '
        .return[]
        | select(.name != "lo" and .name != "lo0")
        | .["ip-addresses"][]? 
        | select(."ip-address-type"=="ipv4")
        | .["ip-address"]
      ' <<<"$out" 2>/dev/null | while read -r ip; do ip_allowed "$ip" && echo "$ip"; done | sort -u | join_csv | trim_commas)
      [[ -n "$ips" ]] && { echo "$ips"; return; }
    fi
  fi
  # Fallback to libvirt
  out=$(virsh domifaddr "$vm" 2>/dev/null | awk 'NR>2 {print $4}' | cut -d/ -f1 | while read -r ip; do ip_allowed "$ip" && echo "$ip"; done | sort -u | join_csv | trim_commas || true)
  [[ -n "${out:-}" ]] && { echo "$out"; return; }
  echo "-"
}

format_ipv4_column () {
  # If --wide, show all; else show first + (+N)
  local csv="$1"
  [[ "$csv" == "-" || -z "$csv" ]] && { echo "-"; return; }
  IFS=',' read -r -a arr <<<"$csv"
  local n=${#arr[@]}
  if [[ $wide_ips -eq 1 || $n -le 1 ]]; then
    echo "$csv"
  else
    printf "%s (+%d)" "${arr[0]}" "$((n-1))"
  fi
}

get_macs () {
  local vm="$1"
  virsh domiflist "$vm" 2>/dev/null | awk 'NR>2 && $0!~/^-+/ {print $5}' | sort -u | join_csv | trim_commas | sed 's/^$/-/'
}

fmt_bytes_gib () { awk '{printf "%.1fGiB", $1/1024/1024/1024}'; }

print_disks_for_vm () {
  local vm="$1"
  local lines
  lines=$(virsh domblklist --details "$vm" 2>/dev/null | awk 'NR>2 && $2=="disk" {print $3, $4, $5}')
  [[ -z "$lines" ]] && { echo "  (no disks)"; return; }

  printf "  %-8s %-8s %-52s %-12s %-12s\n" "TARGET" "BUS" "SOURCE" "CAPACITY" "ALLOCATED"
  while read -r bus target source; do
    if info=$(virsh domblkinfo "$vm" "$target" 2>/dev/null); then
      cap=$(awk -F: '/Capacity/{gsub(/[ \t]/,"",$2); print $2}' <<<"$info" | awk '{print $1}')
      alloc=$(awk -F: '/Allocation/{gsub(/[ \t]/,"",$2); print $2}' <<<"$info" | awk '{print $1}')
      cap_h=$(printf "%s" "$cap" | fmt_bytes_gib)
      alloc_h=$(printf "%s" "$alloc" | fmt_bytes_gib)
    else
      cap_h="?" ; alloc_h="?"
    fi
    printf "  %-8s %-8s %-52s %-12s %-12s\n" "$target" "$bus" "${source:--}" "$cap_h" "$alloc_h"
  done <<< "$lines"
}

# --- Build VM list ---------------------------------------------------------

running_vms=$(virsh list --name | sed '/^$/d' || true)
defined_vms=$(virsh list --all --name | sed '/^$/d' || true)
all_vms=$(printf "%s\n%s\n" "$running_vms" "$defined_vms" | awk 'NF && !seen[$0]++')

# --- Header ----------------------------------------------------------------
printf "%-12s %-4s %-10s %-5s %-8s %-24s %-22s %-17s\n" "NAME" "ID" "STATE" "vCPU" "RAM(MiB)" "HOSTNAME" "IPv4" "MAC(s)"
printf -- "---------------------------------------------------------------------------------------------------------------------------------------------\n"

# --- Rows ------------------------------------------------------------------
for vm in $all_vms; do
  [[ -z "$vm" ]] && continue

  id=$(virsh domid "$vm" 2>/dev/null || true); [[ -z "$id" ]] && id="-"
  state=$(get_dominfo_field "$vm" "State");  [[ -z "$state" ]] && state="-"
  vcpus=$(get_vcpus "$vm");                 [[ -z "$vcpus" ]] && vcpus="-"

  used_mem=$(get_dominfo_field "$vm" "Used memory")
  [[ -z "$used_mem" ]] && used_mem=$(get_dominfo_field "$vm" "Max memory")
  ram_mib=$(to_mib "${used_mem:-0 MiB}")

  hostname=$(get_hostname "$vm")
  ipv4_csv=$(get_ipv4s_all "$vm")
  ipv4_col=$(format_ipv4_column "$ipv4_csv")
  macs=$(get_macs "$vm")

  printf "%-12s %-4s %-10s %-5s %-8s %-24s %-22s %-17s\n" \
    "$vm" "$id" "$state" "$vcpus" "$ram_mib" "$hostname" "$ipv4_col" "$macs"

  if [[ $show_disks -eq 1 ]]; then
    print_disks_for_vm "$vm"
  fi
done

