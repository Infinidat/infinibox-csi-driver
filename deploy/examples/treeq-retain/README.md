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
we will edit the pv-sticky-treeq.yaml file and update the PV parameters as follows:

        volumeAttributes:
        ID: "94199057"
        TREEQID: "20001"
        ipAddress: 172.31.32.158
        storage.kubernetes.io/csiProvisionerIdentity: 1583835085125-8081-infinibox-csi-driver
        storage_protocol: nfs_treeq
        volumePath: /csit_017d653f51/csi-bf4d7e4f43
        volumeHandle: 94199057#20001$$nfs_treeq

This will cause the treeq to be mapped to this PV:

    kubectl create -f ./pv-sticky-treeq.yaml

At this point, you should see two PV's that essentially map to the same treeq on the
ibox:

    kubectl get pv
    NAME              CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS      CLAIM                              STORAGECLASS                          REASON   AGE
    csi-bf4d7e4f43    1Gi        RWX            Retain           Released    infinidat-csi/pvc-existing-pv      ibox-treeq-retain            48m
    sticky-treeq-pv   1Gi        RWX            Retain           Available                                      ibox-treeq-retain            4s

You can now create a PVC that references the sticky-treeq-pv volume:

    kubectl create -f ./pvc-test-sticky.yaml

    kubectl -n infinidat-csiget pvc
    NAME               STATUS   VOLUME            CAPACITY   ACCESS MODES   STORAGECLASS                          AGE
    treeq-sticky-pvc   Bound    sticky-treeq-pv   1Gi        RWX            ibox-treeq-retain   3s

You can test an application can bind to the PVC using the following:

    kubectl create -f ./app-test.yaml

    kubectl -n infinidat-csiget pod
    NAME                              READY   STATUS    RESTARTS   AGE
    infinidat-csi-driver-node-t5r5s   2/2     Running   0          123m
    infinidat-csi-driver-driver-0     5/5     Running   0          123m
    treeq-sticky-pvc-test             1/1     Running   0          4s

    kubectl -n infinidat-csi exec -it treeq-sticky-pvc-test sh
    touch /tmp/data/foo
    echo "foo" > /tmp/data/foo
    cat /tmp/data/foo
    foo
    exit
