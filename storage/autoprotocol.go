package storage

import (
	"fmt"
	"infinibox-csi-driver/api/clientgo"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"os"
	"strings"
)

// determine on this node the protocol based on node
// configuration, this is used when the user specifies
// a protocol service and sets the protocol to 'auto'
// 'auto' is used to pick at runtime the block storage
// that we might support that being (fc, iscsi, or nvme)
// auto is not for nfs or nfs_treeq
func DetermineProtocol() (protocol string, protocolSecret map[string]string, err error) {
	const FN = "DetermineProtocol"
	protocolSecret, protocolSecretInUse, err := helper.GetProtocolSecret()
	if err != nil {
		return "", protocolSecret, fmt.Errorf("%s error: could not get protocol secret %s", FN, err.Error())
	}
	if !protocolSecretInUse {
		return "", protocolSecret, fmt.Errorf("%s error: protocol secret not in use", FN)
	}
	preferredOrder := []string{common.PROTOCOL_FC, common.PROTOCOL_NVME, common.PROTOCOL_ISCSI}
	userPreferredOrder := protocolSecret[common.SC_PROTOCOL_SECRET_AUTO_ORDER]
	if userPreferredOrder != "" {
		preferredOrder = strings.Split(userPreferredOrder, ",")
		zlog.Debug().Msgf("%s user preferred auto order %v", FN, preferredOrder)
	}

	cl, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s  - BuildClient - error: %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return "", protocolSecret, e
	}

	ns := os.Getenv("POD_NAMESPACE")
	zlog.Debug().Msgf("POD_NAMESPACE=%s", ns)
	if ns == "" {
		e := fmt.Errorf("%s - env var POD_NAMESPACE was not set, this is a required env var", FN)
		zlog.Error().Msg(e.Error())
		return "", protocolSecret, e
	}

	pods, err := cl.GetRunningDriverNodePods(ns)
	if err != nil {
		e := fmt.Errorf("%s - GetRunningDriverNodePods - error: %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return "", protocolSecret, e
	}

	zlog.Debug().Msgf("%s found %d driver node pods", FN, len(pods))

	// we only need to test with a single driver node pod since they are required
	// to be configured the same wrt protocol configurations
	podToTest := pods[0].Name

	command := "cat /sys/class/fc_host/ho*/port_state"
	containerName := "driver"
	fcOutput, fcStderr, err := cl.ExecCmdInPod(podToTest, ns, command, containerName)
	zlog.Debug().Msgf("%s fc command stdout [%s] stderr [%s]", FN, fcOutput, fcStderr)
	var fcEnabled bool
	if err != nil {
		zlog.Debug().Msgf("%s fcEnabled set to false due to error %s - stderr %s", FN, err.Error(), fcStderr)
	} else {
		if fcStderr != "" {
			zlog.Debug().Msgf("%s fcEnabled stderr %s , setting to fcEnabled to false", FN, fcStderr)
		} else {
			fcEnabled = isFC(fcOutput)
		}
	}

	var nvmeEnabled bool
	command = "nvme list"
	nvmeOutput, nvmeStderr, err := cl.ExecCmdInPod(podToTest, ns, command, containerName)
	zlog.Debug().Msgf("%s nvme command stdout [%s] stderr [%s]", FN, nvmeOutput, nvmeStderr)
	if err != nil {
		zlog.Debug().Msgf("%s nvmeEnabled set to false due to error %s - stderr %s", FN, err.Error(), nvmeStderr)
	} else {
		if nvmeStderr != "" {
			zlog.Debug().Msgf("%s nvmeEnabled stderr %s , setting to nvmeEnabled to false", FN, nvmeStderr)
		} else {
			nvmeEnabled = isNVME(nvmeOutput)
		}
	}

	//command = "cat /etc/iscsi/initiatorname.iscsi"
	command = "pgrep iscsid"
	var iscsiEnabled bool
	iscsiOutput, iscsiStderr, err := cl.ExecCmdInPod(podToTest, ns, command, containerName)
	zlog.Debug().Msgf("%s iscsi command stdout [%s] stderr [%s]", FN, iscsiOutput, iscsiStderr)
	if err != nil {
		zlog.Debug().Msgf("%s iscsiEnabled set to false due to error %s - stderr %s", FN, err.Error(), iscsiStderr)
	} else {
		if iscsiStderr != "" {
			zlog.Debug().Msgf("%s iscsiEnabled stderr %s , setting to iscsiEnabled to false", FN, iscsiStderr)
		} else {
			iscsiEnabled = isISCSI(iscsiOutput)
		}
	}
	zlog.Debug().Msgf("%s protocol test results [%s=%t] [%s=%t] [%s=%t]", FN, common.PROTOCOL_FC, fcEnabled, common.PROTOCOL_NVME, nvmeEnabled, common.PROTOCOL_ISCSI, iscsiEnabled)

	for _, v := range preferredOrder {
		switch v {
		case common.PROTOCOL_FC:
			if fcEnabled {
				return v, protocolSecret, nil
			}
		case common.PROTOCOL_ISCSI:
			if iscsiEnabled {
				return v, protocolSecret, nil
			}
		case common.PROTOCOL_NVME:
			if nvmeEnabled {
				return v, protocolSecret, nil
			}
		}
	}

	// out of ideas? pick FC and cross fingers
	zlog.Warn().Msgf("%s could not determine protocol based on heuristics, defaulting to FC", FN)
	return common.PROTOCOL_FC, protocolSecret, nil
}

func isFC(output string) bool {
	//read /sys/class/fc_host/host*/port_state and treat Online as usable
	//kubectl exec -it infinidat-csi-driver-node-z5hb6 -c driver -- sh -c "cat /sys/class/fc_host/ho*/port_state"
	// if the word Online is in the output then we assume fc is enabled
	return strings.Contains(output, "Online")
}

func isISCSI(output string) bool {
	// read /etc/iscsi/initiatorname.iscsi
	// pgrep iscsid should return a PID if iscsid is running
	trimmed := strings.TrimSpace(output)
	return trimmed != ""
}

func isNVME(output string) bool {
	return output != ""
}
