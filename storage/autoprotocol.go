package storage

import (
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
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

	fcEnabled := isFC()
	nvmeEnabled := isNVME()
	iscsiEnabled := isISCSI()
	zlog.Debug().Msgf("%s heuristics [%s=%t] [%s=%t] [%s=%t]", FN, common.PROTOCOL_FC, fcEnabled, common.PROTOCOL_NVME, nvmeEnabled, common.PROTOCOL_ISCSI, iscsiEnabled)

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

func isFC() bool {
	//read /sys/class/fc_host/host*/port_state and treat Online as usable
	if validateFCIsOnline() {
		return true
	}
	return false
}

func isISCSI() bool {
	// read /etc/iscsi/initiatorname.iscsi
	// pgrep iscsid should return a PID if iscsid is running
	stdOut, stdErr, err := execCommand.Command("cat", "/etc/iscsi/initiatorname.iscsi")
	if err != nil {
		zlog.Error().Msgf("isISCSI read command error stdout %s stderr %s", stdOut, stdErr)
		return false
	}
	stdOut, stdErr, err = execCommand.Command("pgrep", "iscsid")
	if err != nil {
		zlog.Error().Msgf("isISCSI pgrep command error stdout %s stderr %s", stdOut, stdErr)
		return false
	}
	return true
}

func isNVME() bool {
	stdOut, stdErr, err := execCommand.Command("cat", "/etc/nvme/hostnqn")
	if err != nil {
		zlog.Error().Msgf("isNVME read command error stdout %s stderr %s", stdOut, stdErr)
		return false
	}
	stdOut, stdErr, err = execCommand.Command("nvme", "list")
	if err != nil {
		zlog.Error().Msgf("isNVME nvme list command error stdout %s stderr %s", stdOut, stdErr)
		return false
	}
	return true
}
