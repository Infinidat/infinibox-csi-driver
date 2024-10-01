#!/bin/bash

if [ -z "$CSI_ENDPOINT" ]
then 
    echo "Required: CSI_ENDPOINT is need to be set" 
else 
    socket_file=$CSI_ENDPOINT 
    if [[ $CSI_ENDPOINT == "unix://"* ]]
    then
        socket_file=$(echo $CSI_ENDPOINT | sed 's/^.\{7\}//')
    fi
    [ -e $socket_file ] && rm $socket_file
fi

# Start infinibox-csi-driver with debugging
# enabled per https://github.com/rexray/gocsi
#export X_CSI_DEBUG=true

# Start infinibox-csi-driver or the iboxreplica-controller
if [ -z "$IBOXREPLICA_CONTROLLER" ]
then 
	exec "/infinibox-csi-driver"
else 
	exec "/iboxreplica-controller"
fi

# exec /infinibox-csi-driver $*
