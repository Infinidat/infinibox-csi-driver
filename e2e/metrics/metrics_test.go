//go:build e2e

package metrics

import (
	"context"
	"fmt"
	"infinibox-csi-driver/e2e"
	"io/ioutil"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned/typed/monitoring/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	appName            = "infinidat-csi-metrics"
	serviceMonitorName = appName
	serviceName        = serviceMonitorName
	serviceAccountName = serviceMonitorName
	configMapName      = "infinidat-csi-metrics-config"
)

func TestMetrics(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, _, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}
	config, err := clientcmd.BuildConfigFromFlags("", *e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting k8s config %s", err.Error())
	}

	mClientV1, err := v1monitoringclient.NewForConfig(config)
	if err != nil {
		t.Fatalf("error creatingv1 monitoring client %s", err.Error())
	}

	testNames := setup(t, clientSet, dynamicClient, mClientV1)

	t.Logf("testing in namespace %+v\n", testNames)

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, mClientV1)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func setup(t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, mClientV1 v1monitoringclient.MonitoringV1Interface) (testNames e2e.TestResourceNames) {
	// create a namespace to install into
	ctx := context.Background()
	e2eNamespace := fmt.Sprintf(e2e.E2E_NAMESPACE, "metrics")
	var err error
	uniqueSuffix := e2e.RandSeq(3)
	testNames.NSName, err = e2e.CreateNamespace(ctx, e2eNamespace, uniqueSuffix, client)
	if err != nil {
		t.Fatalf("error setting up e2e namespace %s\n", err.Error())
	}
	t.Logf("✓ Namespace %s is created\n", testNames.NSName)
	// create the ConfigMap

	configFilePath := os.Getenv("CONFIGFILE")
	if configFilePath == "" {
		t.Fatalf("error getting CONFIGFILE %s", err.Error())
	}
	fileContent, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		t.Fatalf("error reading CONFIGFILE %s %s", configFilePath, err.Error())
	}
	configFileData := string(fileContent)
	configFile := map[string]string{
		"config.yaml": configFileData,
	}
	cfg := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapName,
		},
		Data: configFile,
	}
	_, err = client.CoreV1().ConfigMaps(testNames.NSName).Create(ctx, cfg, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating ConfigMap %s\n", err.Error())
	}
	t.Logf("✓ ConfigMap %s is created\n", configMapName)

	// create the Service
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: testNames.NSName,
			Labels: map[string]string{
				"app": appName,
			},
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "web",
					Port:       11007,
					TargetPort: intstr.FromInt(11007),
					Protocol:   "TCP",
				},
			},
			Selector: map[string]string{
				"app": appName,
			},
		},
	}
	_, err = client.CoreV1().Services(testNames.NSName).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating Service %s\n", err.Error())
	}
	t.Logf("✓ Service %s is created\n", serviceName)

	// create the ServiceAccount
	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: testNames.NSName,
		},
	}
	_, err = client.CoreV1().ServiceAccounts(testNames.NSName).Create(ctx, serviceAccount, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating ServiceAccount %s\n", err.Error())
	}
	t.Logf("✓ ServiceAccount %s is created\n", serviceAccountName)

	// create the ClusterRole
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: appName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"persistentvolumes", "secrets"},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{"storage.k8s.io"},
				Resources: []string{"storageclasses"},
				Verbs:     []string{"get", "list"},
			},
		},
	}
	_, err = client.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating ClusterRole %s\n", err.Error())
	}
	t.Logf("✓ ClusterRole %s is created\n", appName)

	// create the ClusterRoleBinding
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: testNames.NSName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     appName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      appName,
				Namespace: testNames.NSName,
			},
		},
	}
	_, err = client.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating ClusterRoleBinding %s\n", err.Error())
	}
	t.Logf("✓ ClusterRoleBinding %s is created\n", appName)

	// create the Deployment

	err = createDeployment(testNames.NSName, client)
	if err != nil {
		t.Fatalf("error creating Deployment %s\n", err.Error())
	}
	t.Logf("✓ Deployment %s is created\n", appName)

	err = e2e.WaitForDeployment(t, appName, testNames.NSName, client)
	if err != nil {
		t.Fatalf("error timed out waiting for Deployment %s to be ready\n", err.Error())
	}
	t.Logf("✓ Deployment %s is ready\n", appName)

	t.Logf("sleeping for 90 seconds\n")

	time.Sleep(time.Second * 90)

	// create the ServiceMonitor (required for Openshift only)
	err = createServiceMonitor(testNames.NSName, mClientV1)
	if err != nil {
		t.Fatalf("could not create ServiceMonitor %s", err.Error())
	}
	t.Logf("✓ ServiceMonitor %s is created\n", serviceMonitorName)

	err = e2e.WaitForDeployment(t, appName, testNames.NSName, client)
	if err != nil {
		t.Fatalf("error timed out waiting for Deployment %s to be ready\n", err.Error())
	}
	t.Logf("✓ Deployment %s is still ready\n", appName)

	return testNames
}

func tearDown(t *testing.T, testNames e2e.TestResourceNames, client *kubernetes.Clientset, dynamicClient dynamic.Interface, mClientV1 v1monitoringclient.MonitoringV1Interface) {
	ctx := context.Background()

	// delete the Deployment
	err := client.AppsV1().Deployments(testNames.NSName).Delete(ctx, appName, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("error deleting Deployment %s\n", err.Error())
	}
	t.Logf("✓ Deployment %s is deleted\n", appName)

	// delete the Service
	err = client.CoreV1().Services(testNames.NSName).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("error deleting Service %s\n", err.Error())
	}
	t.Logf("✓ Service %s is deleted\n", serviceName)

	// delete the ServiceAccount
	err = client.CoreV1().ServiceAccounts(testNames.NSName).Delete(ctx, serviceAccountName, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("error deleting ServiceAccount %s\n", err.Error())
	}
	t.Logf("✓ ServiceAccount %s is deleted\n", serviceAccountName)

	// delete the ClusterRoleBinding
	err = client.RbacV1().ClusterRoleBindings().Delete(ctx, appName, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("error deleting ClusterRoleBinding %s\n", err.Error())
	}
	t.Logf("✓ ClusterRoleBinding %s is deleted\n", appName)

	// delete the ClusterRole
	err = client.RbacV1().ClusterRoles().Delete(ctx, appName, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("error deleting ClusterRole %s\n", err.Error())
	}
	t.Logf("✓ ClusterRole %s is deleted\n", appName)

	// delete the ConfigMap
	err = client.CoreV1().ConfigMaps(testNames.NSName).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("error deleting ConfigMap %s\n", err.Error())
	}
	t.Logf("✓ Configmap %s is deleted\n", configMapName)

	// delete the ServiceMonitor (required for Openshift only)

	err = mClientV1.ServiceMonitors(testNames.NSName).Delete(ctx, serviceMonitorName, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("error deleting ServiceMonitor %s\n", err.Error())
	}
	t.Logf("✓ ServiceMonitor %s is deleted\n", serviceMonitorName)

	// delete the namespace we created in setup

	err = e2e.DeleteNamespace(ctx, testNames.NSName, client)
	if err != nil {
		t.Logf("error deleting namespace %s\n", err.Error())
	}
	t.Logf("✓ Namespace %s is deleted\n", testNames.NSName)
}

func createServiceMonitor(ns string, mClientV1 v1monitoringclient.MonitoringV1Interface) error {
	sm := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceMonitorName,
			Labels: map[string]string{
				"k8s-app": serviceMonitorName,
			},
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": serviceMonitorName,
				},
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:     "web",
					Interval: "30s",
					//BearerTokenSecret: &v1.SecretKeySelector{},
				},
			},
		},
	}
	_, err := mClientV1.ServiceMonitors(ns).Create(context.Background(), sm, metav1.CreateOptions{})
	return err
}

func createDeployment(ns string, client *kubernetes.Clientset) error {
	numReplicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: appName,
			Labels: map[string]string{
				"app": appName,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &numReplicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appName,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": appName,
					},
				},
				Spec: v1.PodSpec{
					ServiceAccountName: appName,
					ImagePullSecrets: []v1.LocalObjectReference{
						{
							Name: "private-docker-reg-secret",
						},
					},
					Containers: []v1.Container{
						{
							Name:  "metrics",
							Image: "git.infinidat.com:4567/host-opensource/infinidat-csi-driver/infinidat-csi-metrics:v2.9.0-rc3",
							VolumeMounts: []v1.VolumeMount{
								{
									Name:      "config-volume",
									ReadOnly:  true,
									MountPath: "/tmp/infinidat-csi-metrics-config",
								},
							},
						},
					},
					Volumes: []v1.Volume{
						{
							Name: "config-volume",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{
										Name: "infinidat-csi-metrics-config",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := client.AppsV1().Deployments(ns).Create(context.Background(), deployment, metav1.CreateOptions{})
	return err
}
