#!/bin/bash
if [[ -z "$_E2E_PROTOCOL" ]]; then
    echo "Must provide _E2E_PROTOCOL in environment" 1>&2
    exit 1
fi
if [[ -z "$_E2E_POOL" ]]; then
    echo "Must provide _E2E_POOL in environment" 1>&2
    exit 1
fi
if [[ -z "$_E2E_NETWORK_SPACE" ]]; then
    echo "Must provide _E2E_NETWORK_SPACE in environment" 1>&2
    exit 1
fi
if [[ -z "$_E2E_NAMESPACE" ]]; then
    echo "Must provide _E2E_NAMESPACE in environment" 1>&2
    exit 1
fi
if [[ -z "$_E2E_OCP_VERSION" ]]; then
    echo "Must provide _E2E_OCP_VERSION in environment" 1>&2
    exit 1
fi
if [[ -z "$OPTIONAL_DOCKER_BUILD_FLAGS" ]]; then
    echo "WARNING: for running tests on the VPN, you will want to provide OPTIONAL_DOCKER_BUILD_FLAGS in environment, example: --network=host" 1>&2
fi
echo $_E2E_PROTOCOL is the protocol we are testing

echo $_E2E_NAMESPACE is e2e namespace

# create working dir
export WORKDIR=`mktemp -d /tmp/redhat-e2e.$_E2E_PROTOCOL.XXXXXX`
echo $WORKDIR

echo $KUBECONFIG was kubeconfig at start

BINDIR=/tmp/redhat-e2e/$_E2E_OCP_VERSION

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

# copy test manifests to working dir
cp $KUBECONFIG $WORKDIR/kubeconfig.yaml
cp $SCRIPT_DIR/ocp-$_E2E_OCP_VERSION-tests-to-run $WORKDIR/tests-to-run
cp $SCRIPT_DIR/e2e-manifest-$_E2E_PROTOCOL.yaml  $WORKDIR/e2e-manifest.yaml
envsubst < $SCRIPT_DIR/e2e-storageclass-$_E2E_PROTOCOL.yaml > $WORKDIR/e2e-storageclass-$_E2E_PROTOCOL.yaml
envsubst < $SCRIPT_DIR/e2e-volume-snapshotclass.yaml > $WORKDIR/e2e-volume-snapshotclass.yaml

export TEST_CSI_DRIVER_FILES=$WORKDIR/e2e-manifest-$_E2E_PROTOCOL.yaml 
echo $OPTIONAL_DOCKER_BUILD_FLAGS are the docker flags
docker run  $OPTIONAL_DOCKER_BUILD_FLAGS -v $WORKDIR:/data:z --rm -it registry.redhat.io/openshift4/ose-tests:$_E2E_OCP_VERSION sh -c "KUBECONFIG=/data/kubeconfig.yaml TEST_CSI_DRIVER_FILES=/data/e2e-manifest.yaml /usr/bin/openshift-tests run openshift/csi --file /data/tests-to-run --junit-dir /data/results"
