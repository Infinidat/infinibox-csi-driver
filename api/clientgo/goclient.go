package clientgo

import (
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type KubeClient interface {
	GetSecret(secretName, nameSpace string) (map[string]string, error)
	GetClusterVerion() (string, error)
}

type kubeclient struct {
	client kubernetes.Interface
}

var clientapi kubeclient

//BuildClient
func BuildClient() (kc *kubeclient) {
	log.Debug("BuildClient called.")
	if clientapi.client == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			log.Error("BuildClient Error while getting cluster config", err)
			panic(err.Error())
		}
		// creates the clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			log.Error("BuildClient Error while creating client", err)
			panic(err.Error())
		}
		clientapi = kubeclient{clientset}
	}
	return &clientapi
}

func (kc *kubeclient) GetSecret(secretName, nameSpace string) (map[string]string, error) {
	log.Debugf("get request for secret with namespace %s and secretname %s", nameSpace, secretName)
	secretMap := make(map[string]string)
	secret, err := kc.client.CoreV1().Secrets(nameSpace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		log.Errorf("Error Getting secret with namespace %s and secretname %s Error: %v ", nameSpace, secretName, err)
		return secretMap, err
	}
	for key, value := range secret.Data {
		secretMap[key] = string(value)
	}
	for key, value := range secret.StringData {
		secretMap[key] = string(value)
	}
	return secretMap, nil
}

func (kc *kubeclient) GetNodeIpsByMountedVolume(volumeName string) ([]string, error) {
	pv, err := kc.GetPersistantVolumeByName(volumeName)
	if err != nil {
		return nil, err
	}
	nameSpace := pv.Spec.ClaimRef.Namespace
	pvcName := pv.Spec.ClaimRef.Name
	pods, err := kc.client.CoreV1().Pods(nameSpace).List(metav1.ListOptions{})
	if err != nil {
		log.Error("error occured", err)
		return nil, err
	}
	nodeIps := []string{}
	for _, p := range pods.Items {
		for _, volume := range p.Spec.Volumes {
			claim := volume.VolumeSource.PersistentVolumeClaim
			if claim != nil && claim.ClaimName == pvcName {
				nodeName, err := kc.GetNodeIdByNodeName(p.Spec.NodeName)
				if err != nil {
					return nil, err
				}
				nodeIps = append(nodeIps, nodeName)
			}
		}
	}
	return nodeIps, nil
}

func (kc *kubeclient) GetPersistantVolumeByName(volumeName string) (*v1.PersistentVolume, error) {
	persistVol, err := kc.client.CoreV1().PersistentVolumes().Get(volumeName, metav1.GetOptions{})
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return persistVol, nil
}

func (kc *kubeclient) GetNodeIdByNodeName(nodeName string) (InternalIp string, err error) {
	node, err := kc.client.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	nodeip := ""
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			nodeip = addr.Address
		}
	}
	return nodeip, err
}

func (kc *kubeclient) GetClusterVerion() (string, error) {
	info, err := kc.client.Discovery().ServerVersion()
	if err != nil {
		log.Error(err)
		return "", err
	}
	return info.GitVersion, nil
}
