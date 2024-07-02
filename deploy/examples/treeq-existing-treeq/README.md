# treeq Example - Existing treeq

This example shows how you can create a PV for a treeq that is on your ibox system,
likely created by an ibox administrator for you.

First, create a PV that maps to that treeq:
    
    kubectl create -f ./pv-existing-treeq.yaml

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

    kbuectl create -f ./pvc-existing-treeq.yaml

You should end up with a bound PVC similar to this:

    kubectl -n infinidat-csiget pvc
    NAME             STATUS   VOLUME              CAPACITY   ACCESS MODES   STORAGECLASS                          AGE
    existing-treeq   Bound    existing-treeq-pv   1Gi        RWX            ibox-treeq-retain   102m

    kubectl -n infinidat-csiget pv
    NAME                CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS     CLAIM                            STORAGECLASS                          REASON   AGE
    existing-treeq-pv   1Gi        RWX            Retain           Bound      infinidat-csi/existing-treeq     ibox-treeq-retain            103m

Lastly, create a sample application that uses the PVC:

    kubectl create -f ./pod-test.yaml

