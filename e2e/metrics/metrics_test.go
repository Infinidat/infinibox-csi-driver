//go:build e2e

package metrics

import (
	"context"
	"errors"
	"infinibox-csi-driver/e2e"
	"os"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned/typed/monitoring/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"k8s.io/client-go/kubernetes"
)

const (
	appName            = "infinidat-csi-metrics"
	serviceMonitorName = appName
	serviceName        = serviceMonitorName
	serviceAccountName = serviceMonitorName
	configMapName      = "infinidat-csi-metrics-config"
)

func TestMetrics(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, "metrics")
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	mClientV1, err := v1monitoringclient.NewForConfig(testConfig.RestConfig)
	if err != nil {
		t.Fatalf("error creatingv1 monitoring client %s", err.Error())
	}

	setup(testConfig, mClientV1)

	t.Logf("testing in namespace %+v\n", testConfig.TestNames)

	if *e2e.CleanUp {
		tearDown(testConfig, mClientV1)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func setup(config *e2e.TestConfig, mClientV1 v1monitoringclient.MonitoringV1Interface) {
	// create a namespace to install into
	ctx := context.Background()
	err := e2e.CreateNamespace(ctx, config.TestNames.NSName, config.ClientSet)
	if err != nil {
		config.Testt.Fatalf("error setting up e2e namespace %s\n", err.Error())
	}
	config.Testt.Logf("✓ Namespace %s is created\n", config.TestNames.NSName)

	err = e2e.CreateImagePullSecret(config.Testt, config.TestNames.NSName, config.ClientSet)
	if err != nil {
		config.Testt.Fatalf("error creating image pull secret %s", err.Error())
	}
	config.Testt.Logf("✓ Image Pull Secret %s is created\n", e2e.IMAGE_PULL_SECRET)

	// create the ConfigMap
	configFilePath := os.Getenv("CONFIGFILE")
	if configFilePath == "" {
		config.Testt.Fatalf("error getting CONFIGFILE %s", err.Error())
	}
	fileContent, err := os.ReadFile(configFilePath)
	if err != nil {
		config.Testt.Fatalf("error reading CONFIGFILE %s %s", configFilePath, err.Error())
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
	_, err = config.ClientSet.CoreV1().ConfigMaps(config.TestNames.NSName).Create(ctx, cfg, metav1.CreateOptions{})
	if err != nil {
		config.Testt.Fatalf("error creating ConfigMap %s\n", err.Error())
	}
	config.Testt.Logf("✓ ConfigMap %s is created\n", configMapName)

	// create the Service
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: config.TestNames.NSName,
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
	_, err = config.ClientSet.CoreV1().Services(config.TestNames.NSName).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		config.Testt.Fatalf("error creating Service %s\n", err.Error())
	}
	config.Testt.Logf("✓ Service %s is created\n", serviceName)

	// create the ServiceAccount
	serviceAccount := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: config.TestNames.NSName,
		},
	}
	_, err = config.ClientSet.CoreV1().ServiceAccounts(config.TestNames.NSName).Create(ctx, serviceAccount, metav1.CreateOptions{})
	if err != nil {
		config.Testt.Fatalf("error creating ServiceAccount %s\n", err.Error())
	}
	config.Testt.Logf("✓ ServiceAccount %s is created\n", serviceAccountName)

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
	_, err = config.ClientSet.RbacV1().ClusterRoles().Get(ctx, appName, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		config.Testt.Logf("ClusterRole %s not found, will create it\n", appName)
		_, err = config.ClientSet.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
		if err != nil {
			config.Testt.Fatalf("error creating ClusterRole %s\n", err.Error())
		}
		config.Testt.Logf("✓ ClusterRole %s is created\n", appName)
	} else if err != nil {
		config.Testt.Fatalf("error getting ClusterRole %s\n", err.Error())
	} else {
		config.Testt.Logf("✓ ClusterRole %s is found\n", appName)
	}

	// create the ClusterRoleBinding
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: config.TestNames.NSName,
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
				Namespace: config.TestNames.NSName,
			},
		},
	}
	err = config.ClientSet.RbacV1().ClusterRoleBindings().Delete(ctx, appName, metav1.DeleteOptions{})
	if err != nil {
		config.Testt.Logf("✓ ClusterRoleBinding delete error %s \n", appName)
	}

	_, err = config.ClientSet.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if err != nil {
		config.Testt.Fatalf("error creating ClusterRoleBinding %s\n", err.Error())
	}
	config.Testt.Logf("✓ ClusterRoleBinding %s is created\n", appName)

	// create the Deployment

	err = createDeployment(config.TestNames.NSName, config.ClientSet)
	if err != nil {
		config.Testt.Fatalf("error creating Deployment %s\n", err.Error())
	}
	config.Testt.Logf("✓ Deployment %s is created\n", appName)

	err = e2e.WaitForDeployment(config.Testt, appName, config.TestNames.NSName, config.ClientSet)
	if err != nil {
		config.Testt.Fatalf("error timed out waiting for Deployment %s to be ready\n", err.Error())
	}
	config.Testt.Logf("✓ Deployment %s is ready\n", appName)

	config.Testt.Logf("sleeping for 90 seconds\n")

	time.Sleep(time.Second * 90)

	// create the ServiceMonitor (required for Openshift only)
	err = createServiceMonitor(config.TestNames.NSName, mClientV1)
	if err != nil {
		config.Testt.Fatalf("could not create ServiceMonitor %s", err.Error())
	}
	config.Testt.Logf("✓ ServiceMonitor %s is created\n", serviceMonitorName)

	err = e2e.WaitForDeployment(config.Testt, appName, config.TestNames.NSName, config.ClientSet)
	if err != nil {
		config.Testt.Fatalf("error timed out waiting for Deployment %s to be ready\n", err.Error())
	}
	config.Testt.Logf("✓ Deployment %s is still ready\n", appName)

}

func tearDown(config *e2e.TestConfig, mClientV1 v1monitoringclient.MonitoringV1Interface) {
	ctx := context.Background()

	// delete the Deployment
	err := config.ClientSet.AppsV1().Deployments(config.TestNames.NSName).Delete(ctx, appName, metav1.DeleteOptions{})
	if err != nil {
		config.Testt.Logf("error deleting Deployment %s\n", err.Error())
	}
	config.Testt.Logf("✓ Deployment %s is deleted\n", appName)

	// delete the Service
	err = config.ClientSet.CoreV1().Services(config.TestNames.NSName).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		config.Testt.Logf("error deleting Service %s\n", err.Error())
	}
	config.Testt.Logf("✓ Service %s is deleted\n", serviceName)

	// delete the ServiceAccount
	err = config.ClientSet.CoreV1().ServiceAccounts(config.TestNames.NSName).Delete(ctx, serviceAccountName, metav1.DeleteOptions{})
	if err != nil {
		config.Testt.Logf("error deleting ServiceAccount %s\n", err.Error())
	}
	config.Testt.Logf("✓ ServiceAccount %s is deleted\n", serviceAccountName)

	// delete the ClusterRoleBinding
	err = config.ClientSet.RbacV1().ClusterRoleBindings().Delete(ctx, appName, metav1.DeleteOptions{})
	if err != nil {
		config.Testt.Logf("error deleting ClusterRoleBinding %s\n", err.Error())
	}
	config.Testt.Logf("✓ ClusterRoleBinding %s is deleted\n", appName)

	// delete the ClusterRole
	err = config.ClientSet.RbacV1().ClusterRoles().Delete(ctx, appName, metav1.DeleteOptions{})
	if err != nil {
		config.Testt.Logf("error deleting ClusterRole %s\n", err.Error())
	}
	config.Testt.Logf("✓ ClusterRole %s is deleted\n", appName)

	// delete the ConfigMap
	err = config.ClientSet.CoreV1().ConfigMaps(config.TestNames.NSName).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil {
		config.Testt.Logf("error deleting ConfigMap %s\n", err.Error())
	}
	config.Testt.Logf("✓ Configmap %s is deleted\n", configMapName)

	// delete the ServiceMonitor (required for Openshift only)

	err = mClientV1.ServiceMonitors(config.TestNames.NSName).Delete(ctx, serviceMonitorName, metav1.DeleteOptions{})
	if err != nil {
		config.Testt.Logf("error deleting ServiceMonitor %s\n", err.Error())
	}
	config.Testt.Logf("✓ ServiceMonitor %s is deleted\n", serviceMonitorName)

	// delete the namespace we created in setup

	err = e2e.DeleteNamespace(ctx, config.TestNames.NSName, config.ClientSet)
	if err != nil {
		config.Testt.Logf("error deleting namespace %s\n", err.Error())
	}
	config.Testt.Logf("✓ Namespace %s is deleted\n", config.TestNames.NSName)
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
	metricsImage := os.Getenv("METRICS_IMAGE")
	if metricsImage == "" {
		return errors.New("METRICS_IMAGE environment variable was not set and is required")
	}
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
							Image: metricsImage,
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
