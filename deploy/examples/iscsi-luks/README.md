# Host-side LUKS encryption of block volumes

Volumes of a StorageClass with `encrypted: "true"` are encrypted on the node with LUKS2
(dm-crypt), with one passphrase per PVC (or per tenant) kept in a Kubernetes Secret. This is
independent of the InfiniBox data-at-rest encryption: the array only ever sees ciphertext, and
the keys stay with whoever controls the Secrets. Snapshots, clones and volume expansion keep
working.

It works with the block protocols: `iscsi`, `fc` and `nvme`, for both `volumeMode: Filesystem`
and `volumeMode: Block`. It is not available for `nfs` and `nfs_treeq`.

## Example

`make setup` in this directory creates the StorageClass, the Secret with a random passphrase,
a PVC and a pod; `make teardown` removes them. Without the Makefile:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ibox-iscsi-luks
provisioner: infinibox-csi-driver
allowVolumeExpansion: true
parameters:
  # ... the usual InfiniBox parameters and secrets, see storageclass.yaml ...
  storage_protocol: "iscsi"
  encrypted: "true"
  encryption_secret_name: "${pvc.name}-luks"
  encryption_secret_namespace: "${pvc.namespace}"
---
apiVersion: v1
kind: Secret
metadata:
  name: data-luks         # "${pvc.name}-luks" for the PVC "data"
  namespace: my-app
type: Opaque
stringData:
  passphrase: "..."       # e.g. openssl rand -base64 32
```

## StorageClass parameters

| Parameter | Value |
|---|---|
| `encrypted` | `"true"` to encrypt the volumes of the StorageClass. Default `"false"`. |
| `encryption_secret_name` | Name of the Secret with the passphrase. Required when `encrypted` is true. |
| `encryption_secret_namespace` | Namespace of that Secret. Required when `encrypted` is true. |

The two Secret parameters accept the templates of the CSI external-provisioner,
`${pvc.name}`, `${pvc.namespace}` and `${pv.name}`, which are replaced when the volume is
created (the chart runs the provisioner with `--extra-create-metadata`, which provides them).
Fixed names work too, for one passphrase shared by all the volumes of the StorageClass.
With `${pvc.namespace}` a PVC can only use a Secret of its own namespace.

The resolved Secret name and namespace are stored in the PV (`spec.csi.volumeAttributes`).
A pre-provisioned PV sets the same three attributes, with the final names.

The Secret must have a `passphrase` key. Its value is used byte for byte, like a cryptsetup
key file: a trailing newline is part of the passphrase (the driver logs a warning when it
finds one). **Keep the passphrase: without it the data cannot be read.**

## How it works

The node plugin reads the Secret with the Kubernetes API (its ClusterRole already allows
reading Secrets) and gives the passphrase to `cryptsetup` on stdin: it is never on a command
line, in a log or in a file.

| CSI call | What happens on the node |
|---|---|
| `NodePublishVolume` | After the multipath (or NVMe) device is attached: if it is not LUKS yet and it is blank, `cryptsetup luksFormat` (LUKS2, aes-xts-plain64, 512-bit key, argon2id); then `cryptsetup open` as `/dev/mapper/infinibox-luks-<volume id>`, which is formatted with `csi.storage.k8s.io/fstype` and mounted (or bind-mounted for a block volume). A second pod on the same node reuses the open device. |
| `NodeUnstageVolume` | `cryptsetup close`, then the multipath device is removed as usual. |
| `NodeExpandVolume` | After the array volume and the multipath device have grown, `cryptsetup resize`, then `resize2fs` / `xfs_growfs`. Online expansion works. |

Safety rules:

- a device that is not LUKS is formatted only if it is blank: `blkid -p` finds no signature
  (filesystem, partition table, LVM, RAID...) and its first and last 8 MiB read as zero. A
  volume with data, for example one cloned from an unencrypted snapshot, is refused and left
  untouched;
- a wrong passphrase fails the mount ("the passphrase does not open the LUKS device" in the
  pod events) and does not modify the device;
- a LUKS device already open on the node is reused only if it sits on the same device and the
  passphrase opens it, so a device left open never lets a wrong passphrase through;
- a volume that asks for encryption is never mounted without it: a missing or invalid Secret
  reference fails the mount.

## Node requirements

- `cryptsetup` 2.x installed on the node (package `cryptsetup` on RHEL, included in RHCOS), and
  the `dm_crypt` kernel module (loaded automatically by `cryptsetup open`). Like the other
  storage tools of the driver, `cryptsetup` runs on the node through `host-chroot.sh`.
- Memory: each key derivation (format, open, resize) uses `LUKS_PBKDF_MEMORY_KIB` of memory
  (default 262144 KiB, 256 MiB; Helm value `luksPbkdfMemoryKiB`), charged to the node server
  container, whose memory limit (`nodeserver.resources.limits.memory`, default 450Mi) must stay
  well above it. The driver runs one key derivation at a time. The value is written in the LUKS
  header when a volume is formatted, so a change only applies to new volumes.

## Things to know

- **Capacity**: the LUKS2 header takes 16 MiB of the volume, so the filesystem (or the block
  device) is 16 MiB smaller than the requested size.
- **Snapshots and clones** carry the LUKS header of their source: the Secret of the new PVC
  must contain the passphrase of the source volume.
- **Changing the passphrase** is not automated: on the node, `cryptsetup luksAddKey` and
  `cryptsetup luksRemoveKey` on the multipath device, then update the Secret.
- **Discard/TRIM** is not passed through the encryption layer, so that the array cannot see
  which blocks are free; space freed in the filesystem is not returned to a thin pool.
- A volume opened read-only on a node (read-only access mode) cannot be used read-write by
  another pod on the same node until it is closed.
- **ReadWriteMany block volumes** (e.g. for live migration of VMs): the first use formats the
  volume, and two nodes using a new volume for the first time at the same moment could both
  format it. Start the first pod alone; once the volume is LUKS it can be opened on many nodes.

## Troubleshooting

Mount errors are in the events of the pod (`kubectl describe pod`) and in the log of the node
server. On the node:

```bash
cryptsetup status infinibox-luks-<volume id>   # open LUKS device of a volume
lsblk                                          # mpathX -> infinibox-luks-<volume id> -> mount point
cryptsetup luksDump /dev/mapper/mpathX         # LUKS header of a volume
```
