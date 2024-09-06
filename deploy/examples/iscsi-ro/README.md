# ro example

This example shows how read only is specified in a Pod spec, when set this will cause
the CSI driver to mount the iscsi volume as read-only.

For this example to work, you have to do the following steps:

 * create the initial iscsi volume/pvc/pod, this will cause data to be written to
the mounted filesystem at /tmp/csitesting/testfile

```
make setup
```

This will create an initial iscsi pod/pvc/pv/storageclass that you will use in the following
steps when you create a read-only pod that references the iscsi volume you have just created.

 * on a single node cluster, delete the writer pod to avoid mount errors when the reader pod runs
```
oc delete pod iscsi-ro-test-writer
```

 * edit the pv-ro.yaml file, updating the following lines with the information that is 
   unique to your ibox host and the volume you just created:

```
      iqn: iqn.2009-11.com.infinidat:storage:infinibox-sn-1111
      portals: 1.1.1.1,1.1.1.2,1.1.1.3
      network_space: "iSCSI"
      storage.kubernetes.io/csiProvisionerIdentity: 1111111111111-3054-infinibox-csi-driver
      volumeHandle: 1204578$$iscsi
```

Tip: Most of these values you can locate from the CSI driver's node log if DEBUG is enabled.

 * create the read-only PV,PVC,Pod


```
make setup-reader
```

## verify things worked

At this point, the reader pod should be running.  It will mount the volume read-only.  You 
can verify this using the following commands:
```
kubectl exec -it iscsi-ro -- mount | grep csitesting
```
In that output, look for the mounted /tmp/csitesting directory, it should show a 'ro' mount option
meaning that the volume was mounted read-only.

You can verify that the volume has data as well:
```
kubectl exec -it iscsi-ro -- cat /tmp/csitesting/testfile
```

Example:
```
09:51:21 ~/infinidat-csi-driver/deploy/examples/iscsi-ro (CSIC-763*) $ oc exec -it iscsi-ro-test -- mount | grep csite
/dev/mapper/mpathak on /tmp/csitesting type xfs (ro,seclabel,relatime,nouuid,attr2,inode64,logbufs=8,logbsize=64k,sunit=128,swidth=2048,noquota)

09:51:40 ~/infinidat-csi-driver/deploy/examples/iscsi-ro (CSIC-763*) $ oc exec -it iscsi-ro-test -- cat /tmp/csitesting/testfile
wwwwwwwwwwwwwww
```

## Cleanup

To clean up, run the following:

```
make teardown
make teardown-reader
```

