# treeq Example 1 - Creating a New treeq

The most basic example is where you dyanmically create a treeq using
a PVC.

First, modify the **example-basic/storageclass.yaml** to match your ibox configuration, you'll
change the following parameters to suit your ibox configuration:

- pool_name
- network_space
- nfs_export_permissions
- namespace

Optionally, you can override ibox limits by setting the following storage class parameters:

- max_filesystems
- max_treeqs_per_filesystem
- max_filesystem_size


Next, create the storage class:

    kubectl create -f example-basic/storageclass.yaml

Note that this storage class, uses a reclaimPolicy of Delete, meaning that if
a PVC is deleted, the PV that it is bound to also get deleted.  You might not want
that sort of policy.  The other treeq example, example-retain-pv, uses a
reclaimPolicy of Retain to demonstrate how PVs, once created, can be retained and reused by
different PVCs.

Next, create the PVC;

    kubectl create -f example-basic/pvc.yaml

When you create a PVC using treeq, if the size you have requested fits within an existing
ibox filesystem, that filesystem will have a treeq created within it for this PVC.  If
the requested size can't fit within an existing ibox filesystem, a new filesystem will be
created to hold this PVC treeq.

At this point you can view the PVC and the PV that were created:

    kubectl get pv
    kubectl -n infinidat-csi get pvc

You should see output similar to this:

    kubectl get pv
    NAME             CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                      STORAGECLASS                   REASON   AGE
    csi-43eaa20364   1Gi        RWX            Delete           Bound    infinidat-csi/             ibox-treeq-pvc-demo   ibox-treeq

    kubectl -n infinidat-csiget pvc
    NAME                  STATUS   VOLUME           CAPACITY   ACCESS MODES   STORAGECLASS                   AGE
    ibox-treeq-pvc-demo   Bound    csi-43eaa20364   1Gi        RWX            ibox-treeq 70m

**Note:  You can use the ibox web application to view the filesystem/treeq that were created.**

Next, create the sample application which will use the PVC;

    kubectl create -f example-basic/app.yaml

Lastly, test the application using 'exec' to get into the application, notice that the PVC is mounted at /tmp/data,
create a sample file on the mounted directory, this tests write access:

    kubectl -n infinidat-csiexec -it ibox-pod-treeq-demo sh
    cd /tmp/data
    touch it
    echo "foo" > /tmp/data/it
    cat /tmp/data/it

# treeq Example - Existing PV

In this example, we are going to show how you can reuse an existing PV.
For this use case, we will create a storage class that has a reclaim policy of **Retain**.  
This will allow PV's to be retained instead of deleted if a PVC is deleted.  This way,
we can create future PVCs that map back to an existing PV.

We will create a storage class that sets the reclaimPolicy to **Retain**:

    kubectl create example-retain-pv/storageclass.yaml

Next, create a PVC which will in turn dynamically create a PV:

    kubectl create -f example-retain-pv/pvc-initialize.yaml

Then, when the PVC is created, take a look at the PV, we want to make note
of the volumeHandle and volume path that was used for the PV:

    kubectl get pv csi-bfd7e4f43 -o yaml

The output will have volumePath and volumeHandle parameter values:

      ID: "94199057"
      TREEQID: "20001"
      gid: ""
      ipAddress: 172.31.32.158
      storage.kubernetes.io/csiProvisionerIdentity: 1669928558050-8081-infinibox-csi-driver
      storage_protocol: nfs_treeq
      uid: ""
      unix_permissions: ""
      volumePath: /csit_017d653f51/csi-bf4d7e4f43
    volumeHandle: 94199057#20001$$nfs_treeq

the volumePath and volumeHandle are unique to the treeq volume on the ibox, we can
later on use this information to construct a new PV that maps to this treeq.  But for
now, lets create a PVC that maps to this PV directly.

Create a PVC that maps to this PV by specifying a volumeName in the PVC that 
matches the PV name from above, inside the **pvc-existing-pv.yaml**, specify
the volume name parameter:

    volumeName:  csi-bfd7e4f43

Then, create the PVC:

    kubectl create -f example-retain-pv/pvc-existing-pv.yaml

Edit the PV, and remove any existing claimRef:

    kubectl edit pv csi-bfd7e4f3

Removing the claimRef will allow the new PVC request to proceed, with the pvc-existing-pv PVC 
claiming the PV.

Next, lets create another PV that maps to the same treeq as used above, for this
we will edit the sticky-treeq-pv.yaml file and update the PV parameters as follows:

        volumeAttributes:
        ID: "94199057"
        TREEQID: "20001"
        ipAddress: 172.31.32.158
        storage.kubernetes.io/csiProvisionerIdentity: 1583835085125-8081-infinibox-csi-driver
        storage_protocol: nfs_treeq
        volumePath: /csit_017d653f51/csi-bf4d7e4f43
        volumeHandle: 94199057#20001$$nfs_treeq

This will cause the treeq to be mapped to this PV:

    kubectl create -f example-retain-pv/sticky-treeq-pv.yaml

At this point, you should see two PV's that essentially map to the same treeq on the
ibox:

    kubectl get pv
    NAME              CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS      CLAIM                              STORAGECLASS                          REASON   AGE
    csi-bf4d7e4f43    1Gi        RWX            Retain           Released    infinidat-csi/pvc-existing-pv      ibox-treeq-retain            48m
    sticky-treeq-pv   1Gi        RWX            Retain           Available                                      ibox-treeq-retain            4s

You can now create a PVC that references the sticky-treeq-pv volume:

    kubectl create -f example-retain-pv/pvc-test-sticky.yaml

    kubectl -n infinidat-csiget pvc
    NAME               STATUS   VOLUME            CAPACITY   ACCESS MODES   STORAGECLASS                          AGE
    treeq-sticky-pvc   Bound    sticky-treeq-pv   1Gi        RWX            ibox-treeq-retain   3s

You can test an application can bind to the PVC using the following:

    kubectl create -f example-retain-pv/app-test.yaml

    kubectl -n infinidat-csiget pod
    NAME                              READY   STATUS    RESTARTS   AGE
    infinidat-csi-driver-node-t5r5s   2/2     Running   0          123m
    infinidat-csi-driver-driver-0     5/5     Running   0          123m
    treeq-sticky-pvc-test             1/1     Running   0          4s

    kubectl -n infinidat-csiexec -it treeq-sticky-pvc-test sh
    touch /tmp/data/foo
    echo "foo" > /tmp/data/foo
    cat /tmp/data/foo
    foo
    exit

# treeq Example - Existing treeq

This example shows how you can create a PV for a treeq that is on your ibox system,
likely created by an ibox administrator for you.

First, create a PV that maps to that treeq:
    
    kubectl create -f example-existing-treeq/existing-treeq-pv.yaml

In the PV manifest, you'll need to change the following values to match
what treeq and file system you want to use:

    volumeAttributes:
      ID: "94199058"
      TREEQID: "20009"
      ipAddress: 172.31.32.158
      storage.kubernetes.io/csiProvisionerIdentity: 1583835085125-8081-infinibox-csi-driver
      storage_protocol: nfs_treeq
      volumePath: /csit_cc3681284b/other-treeq
    volumeHandle: 94199058#20009$$nfs_treeq

In this example, the file system is **csit_cc3681284b** and the treeq is **other-treeq**.
I've given the ID and TREEQID values that are unique, but those values are also
in the **volumeHandle** parameter.  The **ipAddress** is unique to your ibox network.

Next, create a PVC that maps to the PV you just created:

    kbuectl create -f example-existing-treeq/existing-treeq-pvc.yaml

You should end up with a bound PVC similar to this:

    kubectl -n infinidat-csiget pvc
    NAME             STATUS   VOLUME              CAPACITY   ACCESS MODES   STORAGECLASS                          AGE
    existing-treeq   Bound    existing-treeq-pv   1Gi        RWX            ibox-treeq-retain   102m

    kubectl -n infinidat-csiget pv
    NAME                CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS     CLAIM                            STORAGECLASS                          REASON   AGE
    existing-treeq-pv   1Gi        RWX            Retain           Bound      infinidat-csi/existing-treeq     ibox-treeq-retain            103m

Lastly, create a sample application that uses the PVC:

    kubectl create -f example-existing-treeq/app-test.yaml

