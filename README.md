# vm-info

A pretty libvirt VM summary and a [virsh](https://libvirt.org/manpages/virsh.html)-compatible CLI in a single Go binary, built on top of [`digitalocean/go-libvirt`](https://github.com/digitalocean/go-libvirt) and [Cobra](https://github.com/spf13/cobra).

Run it with no subcommand for an at-a-glance table of every domain on the host. Run it with a subcommand to use it as a virsh replacement for everyday tasks (`list`, `dominfo`, `domifaddr`, `start`, `shutdown`, …).

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

`-c/--connect URI` accepts the same URI forms as `virsh -c`:

| URI                                    | Use case                                  |
|----------------------------------------|-------------------------------------------|
| `qemu:///system`                       | local system libvirtd (default)           |
| `qemu:///session`                      | local user-session libvirtd               |
| `qemu+ssh://user@host/system`          | remote libvirtd over SSH                  |
| `qemu+tcp://host[:port]/system`        | remote libvirtd over TCP                  |
| `qemu+tls://host[:port]/system`        | remote libvirtd over TLS                  |
| `qemu+libssh://user@host/system?…`     | remote over libssh with extra options     |

`$LIBVIRT_DEFAULT_URI` is honoured when `--connect` is not given.

## Subcommands

vm-info provides virsh-equivalents for the most common tasks:

| Read-only                                                                                     | Mutating (require `--rw`)                                       |
|-----------------------------------------------------------------------------------------------|-----------------------------------------------------------------|
| `list`, `dominfo`, `dumpxml`, `domid`, `domuuid`, `domhostname`, `domstate`, `vcpucount`      | `start`, `shutdown`, `destroy`, `reboot`, `suspend`, `resume`   |
| `domifaddr`, `domiflist`, `domblklist`, `domblkinfo`, `version`                               | `qemu-agent-command`                                            |

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
