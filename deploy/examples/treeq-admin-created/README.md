# example -  admin created treeqs

This example shows how an ibox administrator can create a filesystem on the ibox, then create treeqs on that filesystem, then
a kube admin can create PV's that map to those treeqs.  Perhaps an example use case for this is if you had a classroom of students
or users and you wanted to provide each of them a slice of a filesystem to use for some personal use.

## Step 1 - ibox work
First, as an ibox admin, create a filesystem, then on that filesystem, create 2 different treeqs, calle them *user1* and *user2*.

Then using the ibox API, look up the filesystem id and the treeq id's:

```
curl --insecure -u "iboxusername:iboxuserpassword" https://ibox000.your.domain/api/rest/filesystems?pool_name=yourpoolname
```

Here is a snippet of output from that command, notice the *id* value, this is the filesystem ID we'll use when we define
the PV's:
```json
   {
      "type": "MASTER",
      "depth": 0,
      "id": 98125215,
      "name": "treeq-filesystem-test",
   }
```

This will query all filesystems in the pool named *yourpoolname*, from that output you can get the newly
created filesystem ID.

Then with the filesystem ID you can query all the treeqs for it using:

```
curl --insecure -u "iboxusername:iboxuserpassword" https://ibox000.your.domain/api/rest/filesystems/98125215/treeqs
```

This curl command is specifying a filesystem ID of *98125215*.  The output of that command will show you the
treeqs and their treeq ID.  Make not of the treeq IDs because you'll need those in the next step.

Here is a snippet of output from that command, notice the *id* values, this is the treeq ID we'll use when we define
the PV's:
```json
    {
      "id": 20000,
      "filesystem_id": 98125215,
      "name": "user1",
      "path": "/user1",
      "soft_capacity": 1073741824,
      "soft_inodes": null,
      "hard_capacity": 2147483648,
      "hard_inodes": null,
      "used_capacity": 65664,
      "used_inodes": 3,
      "capacity_state": "BELOW_SOFT",
      "inodes_state": "NONE"
    },
    {
      "id": 20001,
      "filesystem_id": 98125215,
      "name": "user2",
      "path": "/user2",
      "soft_capacity": 1073741824,
      "soft_inodes": null,
      "hard_capacity": 2147483648,
      "hard_inodes": null,
      "used_capacity": 65664,
      "used_inodes": 3,
      "capacity_state": "BELOW_SOFT",
      "inodes_state": "NONE"
    }
```

## Step 2 - Create PVs on the Kube Cluster for Each Treeq

Modify pv-user1.yaml, replace the filesystem ID and treeq ID with the values from Step 1.

Modify pv-user2.yaml, replace the filesystem ID and treeq ID with the values from Step 2.

In each PV manifest file, specify the filesystem ID and treeq IDs from above, also specify the IP Address to
your ibox network space, example for pv-user1.yaml looks like the following:

```yaml
    volumeAttributes:
      ipAddress: 192.168.32.158
      volumePath: /treeq-filesystem-test/user1
    volumeHandle: 98125215#996697$$nfs_treeq
```

In that example, the filesystem ID is *98125215*, the treeq name is *user1*, the treeq ID for *user1* is *996697*, the IP address of the NAS network space is *192.168.32.158*.

Create each PV:

```
kubectl create -f pv-user1.yaml
kubectl create -f pv-user2.yaml
```

Then create the PVC for each PV:

```
kubectl create -f pvc-user1.yaml
kubectl create -f pvc-user2.yaml
```

## Step 3 - Create Applications

Lastly, create 2 application pods, one for each treeq:

```
kubectl create -f app-user1.yaml
kubectl create -f app-user2.yaml
```

## Cleanup

Note that the ibox will prevent you from removing a treeq if there is an export for the filesystem.  You can remove the export, then remove the filesystem which will then allow for the removal of the treeq(s).

Alternatively, you can mount the treeq, and manually remove any files on it, then the ibox will let you remove an individual treeq and still retain the filesystem/export that holds it.

