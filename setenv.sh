#! /bin/bash -x
# Start infinibox-csi-driver
cp /etc/multipath.conf /host/etc/multipath.conf
exec /infinibox-csi-driver $*