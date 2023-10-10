
# Overview - Metrics

You can run the infinidat-csi metrics on Openshift or on plain kubernetes.  The steps are a bit
different depending on which platform you want to install upon.

## Configuration

**config.yaml**

This file lets you specify the duration in between metrics collection for
groups of metrics.  For example, specifying '60s' means to create a metric every
60 seconds.

The values for the duration are in golang duration format, documented here:

https://pkg.go.dev/maze.io/x/duration

Examples:
 - 60s - 60 seconds
 - 1h - 1 hour
 - 1w - 1 week
 - 2m - 2 months
 - 3d - 3 day

**Metric Groups**

Metrics are grouped together and referenced in the configuration
as follows:

The groups of metrics are named:

**pool_metrics**, includes the following metrics:
 - ibox_pool_available_cap
 - ibox_pool_used_cap
 - ibox_pool_pct_utilized

**pv_metrics**, includes the following metrics:
 - ibox_pv_total_size

**ibox_performance_metrics**, includes the following metrics:
 - ibox_perf_iops
 - ibox_perf_throughput
 - ibox_perf_latency

**ibox_system_metrics**, includes the following metrics:
 - ibox_active_cache_ssd_devices
 - ibox_active_drives
 - ibox_active_encrypted_cache_ssd_devices
 - ibox_active_encrypted_drives
 - ibox_bbu_aggregate_charge_percent
 - ibox_bbu_charge_level
 - ibox_bbu_protected_nodes
 - ibox_enclosure_failure_safe_distribution
 - ibox_encryption_enabled
 - ibox_failed_drives
 - ibox_inactive_nodes
 - ibox_missing_drives
 - ibox_node_bbu_protection
 - ibox_phasing_out_drives
 - ibox_raid_groups_pending_rebuild_1
 - ibox_raid_groups_pending_rebuild_2
 - ibox_ready_drives
 - ibox_rebuild_1_inprogress
 - ibox_rebuild_2_inprogress
 - ibox_testing_drives
 - ibox_unknown_drives

For more details on these metrics see the Infinibox CSI User Guide.

**cluster-monitoring-config.yaml**

This resource is required for Openshift, create it as-is a single time if not already
created on your cluster.

**servicemonitor.yaml**

This is the ServiceMonitor resource that defines the infinidat-csi metrics service
which is required for Openshift.  Create this a single time on your cluster.

**deploy.yaml**

This is the Deployment that runs the infinidat-csi-metrics container.  This Deployment is installed into the infinidat-csi namespace and will have a single Pod that is running when installed.  This YAML file also includes a Service which maps to port 11007.

The Deployment references a ServiceAccount as well, this ServiceAccount grants RBAC permissions to the metrics container.  A ClusterRole is defined specifically for the metrics collector as well, this ClusterRole is bound by a ClusterRoleBinding to the ServiceAccount.

## Installation on Openshift - setting up infinidat-csi-metrics

Openshift has prometheus installed by default.  To deploy the infinidat CSI metrics exporter, execute 
the steps described in deploy/metrics/README.md

You will create the following resources on your openshift instance in order
to deploy the infinidat-csi-metrics capability and be able to view them
in the openshift web console:

```bash
oc create configmap infinidat-csi-metrics-config --from-file=./config.yaml
oc create -f cluster-monitoring-config.yaml  
oc create -f deploy.yaml  
oc create -f servicemonitor.yaml
```

### Openshift- Viewing infinidat-csi-metrics

You can now query the various infinidat-csi-metrics looking for metrics that
begin with **infinidat_csi_**

### Openshift - Uninstall infinidat-csi Metrics

oc delete configmap infinidat-csi-metrics-config 
oc delete -f cluster-monitoring-config.yaml  
oc delete -f deploy.yaml  
oc delete -f servicemonitor.yaml

## Installation on k8s - Installing via Helm Chart
The prometheus community provides a helm chart you can use, add it as a repo using the following:

```
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
```

Create a namespace for prometheus to be installed into:

```
kubectl create ns prom
```

Deploy the prometheus stack, note that the values.yaml file configures prometheus to scrape the infinidat-csi
metrics:

```
cd deploy/metrics
helm install -f values.yaml  prometheus prometheus-community/kube-prometheus-stack -n prom
```

View the deployment:

```
kubectl get all -n prom
```

### k8s - Install CSI Metrics Collectors

```
kubectl create configmap infinidat-csi-metrics-config --from-file=./config.yaml
kubectl create -f deploy.yaml
```

### k8s - Access Prometheus Dashboard

```
kubectl port-forward -n prom prometheus-prom-kube-prometheus-stack-prometheus-0 9090
```

### k8s - Access Grafana Dashboard

default user/password is admin/prom-operator

```
kubectl port-forward -n prom prom-grafana-6c578f9954-rjdmk 3000
```

Uninstall prometheus:

```
helm uninstall prometheus -n prom
kubectl delete crd alertmanagerconfigs.monitoring.coreos.com
kubectl delete crd alertmanagers.monitoring.coreos.com
kubectl delete crd podmonitors.monitoring.coreos.com
kubectl delete crd probes.monitoring.coreos.com
kubectl delete crd prometheuses.monitoring.coreos.com
kubectl delete crd prometheusrules.monitoring.coreos.com
kubectl delete crd servicemonitors.monitoring.coreos.com
kubectl delete crd thanosrulers.monitoring.coreos.com
```

### k8s - Configure Prometheus to Scrap infinidat-csi-metrics

add this to the prometheus configuration, this is already added to the value.yaml file in this directory for you if 
you intend to install using helm:

```yaml
  - job_name: 'infinidat-csi'
    static_configs:
    - targets: ['infinidat-csi-metrics.infinidat-csi.svc.cluster.local:11007]
```

## Debugging Metrics

you can verify that metrics are getting collected/produced by curl-ing the metrics service,


```
kubectl port-forward service/infinidat-csi-metrics 11007:11007
```

then in another terminal run:

```
curl localhost:11007/metrics
```

view the configured targets, you should see one for infinidat-csi:

```
curl localhost:11007/targets
```

