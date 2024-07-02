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

