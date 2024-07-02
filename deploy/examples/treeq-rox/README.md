# treeq test for ROX ReadOnlyMany access modes

## step 1 - set up and test the admin treeq example

## step 2 - delete the user1 and user2 application pods

```
kubectl delete pod user1-app
kubectl delete pod user2-app
```

## step 3 - delete the user1 and user2 PVCs

```
kubectl delete pvc user1-pvc
kubectl delete pvc user2-pvc
```

## step 4 - edit the user1-pv PV

Remove the older claimRef since we will be creating a new PVC that
will become the new claimRef.

```
kubectl edit pv user1-pv
```

Change the accessMode to ReadOnlyMany (ROX) in the user1-pv by 
editing as above.

Save the changes to the PV.

## step 5 - create a new PVC

```
kubectl create -f pvc-user1.yaml
```

Verify that the new user1 pvc has ROX access mode.

```
kubectl get pvc user1-pvc
```

## step 6 - create the ROX application pod

```
kubectl create -f pod-user1-rox.yaml
```

Verify that the Pod is running and that the filesystem is mounted
readOnly.

```
kubectl exec -it user1-rox-app cat /proc/mounts
```

