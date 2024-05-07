
# treeq with ro specified on volumeMount

here are the steps required to test readOnly "true" on a volumeMount
with a treeq volume.

## step 1 - setup the admin example

This will require the treeq admin example to be setup and tested

## step 2 - delete the user1 and user2 pods

## step 3 - create the readOnly pods

```
kubectl create -f user1-ro-app.yaml
```

Verify the application has started, and then verify that the pod
has a readonly mount:
```
kubectl exec -it user1-ro-app cat /proc/mounts 
```

Verify that the PVC is RWO readwriteonce:
```
kubectl get pvc user1-pvc
```
