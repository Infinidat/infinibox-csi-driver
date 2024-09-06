# static provisioning of volumegroupsnapshot


## step 0 - create snap group on the ibox

 * Create a Consistency Group, lets call it 'dogbert'.
 * Then add an iscsi or fc volume to that Consistency Group.
 * then create a Snap Group for that Consistency Group, call it 'static-test-snap-group'.

When you do that, it will cause a snapshot to be created for the volume.  In this case, the
snapshot is called 'static-testcsi130-2dc903db1a'.

Next, we are going to use static provisioning to create Kube resources that map to these ibox
resources.  

Tip:
static provisioning is going from ibox resources to k8s resources.
dynamic provisioning is going from k8s resources to ibox resources.

## step 1 - create a VolumeGroupSnapshotContent 

look at content.yaml, fill that out with existing ibox snap group information

Here are things you have to look up on the ibox and use to fill out the 
content-example.yaml:

```
  source:
    groupSnapshotHandles:
      volumeGroupSnapshotHandle: "138869243"
      volumeSnapshotHandles:
      - 138869241$$iscsi
  volumeGroupSnapshotClassName: infinibox-groupsnapclass
```

In this snippet, the value "138869243" is the snap group ID, you can find this
by looking at the output from the ibox api with this REST call:
```
curl --insecure -u "username:password" https://ibox000.example.com/api/rest/cgs | jq > ./all-cgs
```

The value "138869241$$iscsi" is the handle for an iscsi snapshot volume created by the snap group.  You can 
look up that by looking thru the output of the following ibox api REST call:

```
curl --insecure -u "username:password" https://ibox000.example.com/api/rest/volumes?pool_name=csitesting | jq > ./all-volumes
```

The value 'infinibox-groupsnapclass' is the VolumeGroupSnapshotClass that is found in this example folder.  You
will need to create that resource if not created already.  Create it by the following:
```
kubectl create -f volumegroupsnapshotclass.yaml
```

After filling out the content-example.yaml file, you can now attempt creating the
VolumeGroupSnapshotContent resource as follows:

```
kubectl create -f content-example.yaml
kubectl get volumegroupsnapshotcontent
```

Verify that the VolumeGroupSnapshotContent you just created is in a ready status!


