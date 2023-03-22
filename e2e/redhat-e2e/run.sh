#!/bin/sh
echo 'Running redhat openshift/csi tests on nfs'
PROTOCOL=nfs
echo $PROTOCOL is the protocol we are testing

export NAMESPACE=openshift-operators
export NETWORKSPACE=NAS
export POOLNAME=csitesting

BRANCH=release-4.11

# create working dir
export WORKDIR=`mktemp -d /tmp/redhat-e2e.$PROTOCOL.XXXXXX`
echo $WORKDIR

export KUBECONFIG=/home/jeffmc/k8screds/csi-training.kubeconfig
BINDIR=/tmp/redhat-e2e/$BRANCH

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# copy test manifests to working dir
envsubst < $SCRIPT_DIR/e2e-manifest-$PROTOCOL.yaml > $WORKDIR/e2e-manifest-$PROTOCOL.yaml
envsubst < $SCRIPT_DIR/e2e-storageclass-$PROTOCOL.yaml > $WORKDIR/e2e-storageclass-$PROTOCOL.yaml
envsubst < $SCRIPT_DIR/e2e-volume-snapshotclass.yaml > $WORKDIR/e2e-volume-snapshotclass.yaml

export TEST_CSI_DRIVER_FILES=$WORKDIR/e2e-manifest-$PROTOCOL.yaml 
#$WORKDIR/openshift-tests run openshift/csi --junit-dir /data/results_iscsi_all"
$BINDIR/openshift-tests run openshift/csi --file $SCRIPT_DIR/tests-to-run --junit-dir $WORKDIR/results_$PROTOCOL_all
