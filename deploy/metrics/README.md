# setting up infinidat-csi-metrics

You will create the following resources on your openshift instance in order
to deploy the infinidat-csi-metrics capability and be able to view them
in the openshift web console:

```bash
oc create configmap infinidat-csi-metrics-config --from-file=./config.yaml
oc create -f cluster-monitoring-config.yaml  
oc create -f deploy.yaml  
oc create -f servicemonitor.yaml
```

## Viewing infinidat-csi-metrics

You can now query the various infinidat-csi-metrics looking for metrics that
begin with **infinidat_csi_**


