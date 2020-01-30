#! /bin/bash -x


 if [ -n "${ISCSI_INITIATOR_NAME}" ]; then
	echo "InitiatorName=${ISCSI_INITIATOR_NAME}" > /etc/iscsi/initiatorname.iscsi 
	# Start iscsid
	iscsid -f &
 fi
rpcbind
# # Start infinibox-csi-driver
/infinibox-csi-driver $*