# vm-info

A pretty VM summary and a [virsh](https://libvirt.org/manpages/virsh.html)-compatible CLI for **libvirt** and **Proxmox VE** in a single Go binary, built on [`digitalocean/go-libvirt`](https://github.com/digitalocean/go-libvirt), the Proxmox VE REST API, and [Cobra](https://github.com/spf13/cobra).

Run it with no subcommand for an at-a-glance table of every domain on the host or Proxmox cluster. Run it with a subcommand to use it as a virsh replacement for everyday tasks (`list`, `dominfo`, `domifaddr`, `start`, `shutdown`, …) — the same subcommands work against both backends.

```text
$ vm-info -c qemu+ssh://user@hv/system
NAME     ID  STATE    vCPU  RAM(MiB)  HOSTNAME  IPv4             MAC(s)
----     --  -----    ----  --------  --------  ----             ------
cp1      2   running  4     8192      cp1       192.168.122.27   52:54:00:34:91:e4
worker1  3   running  4     16384     worker1   192.168.122.65   52:54:00:7b:4a:30
worker2  4   running  4     16384     worker2   192.168.122.133  52:54:00:d5:80:c9
worker3  1   running  4     16384     worker3   192.168.122.92   52:54:00:7d:f6:02
```

## Install

**Pre-built binaries** — grab the archive for your OS/arch from the [releases page](https://github.com/dragonsecurity-incubating/vm-info/releases) and extract:

```sh
tar -xzf vm-info-<version>-linux-amd64.tar.gz
sudo install -m 0755 vm-info-<version>-linux-amd64 /usr/local/bin/vm-info
```

**From source** (Go 1.24+):

```sh
git clone https://github.com/dragonsecurity-incubating/vm-info
cd vm-info
make install                  # installs to /usr/local/bin
make install PREFIX=$HOME/.local
```

Or `go install github.com/dragonsecurity-incubating/vm-info@latest`.

## Connecting

`-c/--connect URI` selects the backend by scheme:

### libvirt

| URI                                    | Use case                                  |
|----------------------------------------|-------------------------------------------|
| `qemu:///system`                       | local system libvirtd (default)           |
| `qemu:///session`                      | local user-session libvirtd               |
| `qemu+ssh://user@host/system`          | remote libvirtd over SSH                  |
| `qemu+tcp://host[:port]/system`        | remote libvirtd over TCP                  |
| `qemu+tls://host[:port]/system`        | remote libvirtd over TLS                  |
| `qemu+libssh://user@host/system?…`     | remote over libssh with extra options     |

### Proxmox VE

```sh
export PVE_API_TOKEN='root@pam!vminfo=00000000-1111-2222-3333-444444444444'
vm-info -c 'pve://pve.example.com/'
# self-signed cert? add ?insecure=1
vm-info -c 'pve://192.168.10.20/?insecure=1'
# token inline (e.g. ad-hoc, no env)
vm-info -c 'pve://10.0.0.1/?token=root@pam!ro=...&insecure=1'
```

vm-info uses [API tokens](https://pve.proxmox.com/wiki/User_Management#pveum_tokens) (no password). Create one in the Proxmox UI under *Datacenter → Permissions → API Tokens*, give it `VM.Audit` (and `VM.PowerMgmt` if you'll use `--rw`), and pass `USER@REALM!TOKENID=SECRET`. A single connection covers the whole cluster — vm-info uses `/cluster/resources` so VMs from every node show up in one table.

Connection-URI resolution order when `--connect` is omitted:

1. `$VM_INFO_URI`
2. `$LIBVIRT_DEFAULT_URI`
3. `qemu:///system`

## Subcommands

vm-info provides virsh-equivalents for the most common tasks. All subcommands work against both backends; a few have backend-specific notes:

| Read-only                                                                                     | Mutating (require `--rw`)                                       |
|-----------------------------------------------------------------------------------------------|-----------------------------------------------------------------|
| `list`, `dominfo`, `dumpxml`, `domid`, `domuuid`, `domhostname`, `domstate`, `vcpucount`      | `start`, `shutdown`, `destroy`, `reboot`, `suspend`, `resume`   |
| `domifaddr`, `domiflist`, `domblklist`, `domblkinfo`, `version`                               | `qemu-agent-command`                                            |
| `snapshot list`, `backup list`                                                                | `snapshot {create,delete,revert}`, `backup {create,delete}`     |

Backend-specific notes:

- `dumpxml` prints libvirt domain XML for `qemu://` and the Proxmox config dictionary for `pve://`.
- `domifaddr --source` accepts `lease`, `agent`, `arp` on libvirt; only `agent` on Proxmox (Proxmox doesn't expose lease/ARP via the API).
- `qemu-agent-command` takes the same `{"execute":"…"}` envelope on both backends; on Proxmox it's translated to a REST `/agent/<cmd>` call.
- `domid` is the libvirt domain ID for libvirt and the VMID for Proxmox.

### Snapshots

Snapshots are point-in-time copies; both backends support them natively.

```sh
vm-info snapshot list cp1
vm-info --rw snapshot create cp1 pre-upgrade --description "before kernel bump"
vm-info --rw snapshot create cp1 with-mem --memory       # include RAM state
vm-info --rw snapshot revert cp1 pre-upgrade
vm-info --rw snapshot delete cp1 pre-upgrade
```

The current snapshot is marked with `*` in `snapshot list`.

### Backups

| Backend  | Status                                                                                              |
|----------|-----------------------------------------------------------------------------------------------------|
| Proxmox  | Full support via `vzdump`; choose mode (snapshot / suspend / stop), compression, target storage.    |
| libvirt  | Not implemented — libvirt's native backup API is incremental-only and requires NBD setup. Use external tooling (`virt-backup`, `virsh backup-begin`) or `snapshot create` for point-in-time copies. |

```sh
vm-info backup list 100
vm-info --rw backup create 100 --storage local --mode snapshot --compress zstd
vm-info --rw backup delete 100 'local:backup/vzdump-qemu-100-...vma.zst'
```

`backup create` returns immediately with the Proxmox task UPID; follow it from the same CLI:

```sh
upid=$(vm-info --rw -c "$pve" backup create 100 --storage local | awk '{print $NF}')
vm-info -c "$pve" task watch "$upid"     # tails the log, exits non-zero on failure
vm-info -c "$pve" task status "$upid"    # one-shot status
vm-info -c "$pve" task log "$upid"       # full log
```

`task` works only against Proxmox UPIDs — libvirt operations are synchronous over RPC, so the libvirt provider returns `not supported` for the whole subtree.

vm-info is **read-only by default** for safety; mutating subcommands refuse to run unless you also pass `--rw`:

```sh
vm-info shutdown cp1            # error: requires --rw
vm-info --rw shutdown cp1       # actually runs
```

### Useful flags on the default table

```sh
vm-info --disks                          # also print each VM's disks
vm-info --wide                           # show all IPv4s, no truncation
vm-info --filter-cidr 10.244.0.0/16      # hide IPs in this range (repeatable)
```

## Build

```sh
make build                               # host platform → ./vm-info
make release                             # cross-compile linux+darwin × amd64+arm64 → dist/
make release PLATFORMS="linux/amd64"     # narrow the matrix
make test vet fmt tidy clean             # the usual
```

Version metadata is injected at link time:

```sh
$ vm-info version
vm-info v0.1.0 (abc1234)
  built: 2026-05-07T19:12:53Z
  go:    go1.24 linux/amd64
```

## Releasing

The `Release` workflow runs on `v*` tags. To cut a release:

```sh
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions cross-compiles for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, packages each binary as `.tar.gz`, writes `SHA256SUMS`, and publishes a GitHub release.

## License

TBD.
