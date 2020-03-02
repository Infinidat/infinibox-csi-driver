#! /bin/bash -x
#  if [ -n "${ISCSI_INITIATOR_NAME}" ]; then
# 	# echo "InitiatorName=${ISCSI_INITIATOR_NAME}" > /etc/iscsi/initiatorname.iscsi 
# 	# sed -i 's/node.startup = manual/#node.startup = manual/' /etc/iscsi/iscsid.conf
# 	# sed -i 's/#node.startup = automatic/node.startup = automatic/' /etc/iscsi/iscsid.conf
# 	# Start iscsid
# 	# exec iscsid -f &
# 	# Start rpcbind
	
#  fi
rpcbind
# Start infinibox-csi-driver
exec /infinibox-csi-driver $*