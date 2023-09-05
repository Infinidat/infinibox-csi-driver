
# Overview - Metrics

You can run the infinidat-csi metrics on Openshift or on plain kubernetes.  The steps are a bit
different depending on which platform you want to install upon.

## Openshift - setting up infinidat-csi-metrics

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

## Openshift- Viewing infinidat-csi-metrics

You can now query the various infinidat-csi-metrics looking for metrics that
begin with **infinidat_csi_**

## Openshift - Uninstall infinidat-csi Metrics

oc delete configmap infinidat-csi-metrics-config 
oc delete -f cluster-monitoring-config.yaml  
oc delete -f deploy.yaml  
oc delete -f servicemonitor.yaml

## k8s - Installing via Helm Chart
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

## k8s - Install CSI Metrics Collectors

```
kubectl create configmap infinidat-csi-metrics-config --from-file=./config.yaml
kubectl create -f deploy.yaml
```

## k8s - Access Prometheus Dashboard

```
kubectl port-forward -n prom prometheus-prom-kube-prometheus-stack-prometheus-0 9090
```

## k8s - Access Grafana Dashboard

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

## k8s - Configure Prometheus to Scrap infinidat-csi-metrics

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

