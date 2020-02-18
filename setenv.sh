#! /bin/bash -x

 if [ -n "${ISCSI_INITIATOR_NAME}" ]; then
	echo "InitiatorName=${ISCSI_INITIATOR_NAME}" > /etc/iscsi/initiatorname.iscsi 
	# Start iscsid
	iscsid -f &
	# Start rpcbind
	rpcbind
 fi
 
# Start infinibox-csi-driver
exec /infinibox-csi-driver $*