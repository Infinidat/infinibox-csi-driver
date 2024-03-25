
# read-only implementation notes

## testing scenario

 * create storageclass with RWO and Retain
 * create PVC with RWO
 * create app POD with RWO
 * let it run to write some data to the volume
 * delete the writer app pod
 * delete the writer pvc
 * verify that the PV has been retained and it will have RWO access mode
 * lookup details about that volume on the ibox using a curl script, we are going to create a read-only PV from this info later on
```
example curl command:
curl --insecure -u "username:password" https://ibox0000/api/rest/volumes?pool_name=csitesting | jq > /tmp/vols
```
In that output, you look for the volume name that was created by the read-write PV.  Then you will see the volume ID, copy
that and paste it into the pv-rox.yaml example where the volume ID is specified.
 * lookup the iqn in the ibox on an existing iscsi volume, paste that into the pv-rox.yaml example where iqn is specified
 * lookup the csi provisioner value in an existing iscsi volume by looking at the read-write PV yaml output, copy that
csi provisioner value and paste it into the pv-rox.yaml example.
 * create the PV using the pv-rox.yaml
 * verify the PV was created with ROX access mode
 * create the read-only PVC using pvc-rox.yaml
 * create the read-only application pod using app-rox.yaml
 * verify the app pod mounts the volume, look at the pod's log and verify it is reading the mounted file created
earlier by the read-write application pod

## Why can't I specify a read-only PVC using an existing volume as a volumeName in the PVC?

The original PV created by the writer has RWO access mode, so when you try to create a ROX PVC it will conflict with the
original PV's access mode and won't work.  So, instead, you create a PV with ROX mode that points to the original volume.  That
way, the ROX PVC can refer to a PV with ROX access mode.

## Why does Openshift ROX pods require the SecurityContext set spc_t?

Since the volume is mounted read-only, selinux can't perform its normal relabeling of the volume as it normally does.  So the
workaround is to specify the SecurityContext value to cause selinux to not perform the relabeling which should be fine
since the volume is read-only at that point and should have already been labeled correctly by the writer pod.

Here is the security context fragment that needs to be added to the read-only application pod:
```yaml
spec:
  securityContext:
    seLinuxOptions:
      type: "spc_t"
```


## The csitestimage is modified to support read-only file systems

Since we need a way to test read-only file systems, the csitestimage has been modified to allow a new environment variable READ_ONLY
be set to 'true' in order to cause the csitestimage to only open up the volume as read-only and just read the file contents created
by a read-write csitestimage prevsiously.

# Why can't the reader pods run on the same kube node as the writer?

The driver code does a lookup using the LUN id in order to find a current mpath device.  So, if the LUNs are the same which they
would be given the read-write/read-only scenario, then the lookup will find the same mpath device twice.  Then, when the driver
tries to mount the mpath device a 2nd time (read-only pod), the mount fails with the 'already mounted' error message.  This is because you normally can't remount a filesystem as read-only if processes have a file on it that's open for writing.  It is possible to mount a mpath device more than once on a single node if its read-write mounted.  So, its the responsiblity of the user to insure
that a read-write mount only happens on a single node, and use node anti-affinity or similar to run read-only mounts on other nodes.

# In read-only volumes, why can't chown/chmod be run by the driver as they normally do?

Once the file system is mounted readonly, then chmod/chown commands don't work, and you would not want them to work since
that is the idea of read-only, being that you don't want to change the current permissions of that volume contents.  So
we skip the chmod/chown assuming that the original volume writer set the permissions as intended on the contents and that the
reader pods run with the same UID/GID as the writer pods.  Also, if you changed the shared volume permissions, you might break
the writer pod.

