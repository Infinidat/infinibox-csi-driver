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
# Start infinibox-csi-driver
exec "/infinibox-csi-driver"

# exec /infinibox-csi-driver $*
