#!/bin/sh
kubectl exec infinidat-csi-driver-driver-0 -c driver -- mktemp -d /tmp/csi-sanity.XXXXXX
