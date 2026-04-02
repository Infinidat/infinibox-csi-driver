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
if [[ "$IBOXREPLICA_CONTROLLER" == "true" ]]; then
	exec "/iboxreplica-controller" $*
elif [[ "$IBOXPROMOTE_CONTROLLER" == "true" ]]; then
	exec "/iboxpromote-controller" $*
elif [[ "$IBOXCG_CONTROLLER" == "true" ]]; then
	exec "/iboxcg-controller" $*
else 
	exec "/infinibox-csi-driver" $*
fi

# exec /infinibox-csi-driver $*
