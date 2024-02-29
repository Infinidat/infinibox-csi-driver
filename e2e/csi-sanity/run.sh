#!/bin/bash

if [[ -z "$_E2E_IBOX_HOSTNAME" ]]; then
    echo "Must provide _E2E_IBOX_HOSTNAME in environment" 1>&2
    exit 1
fi
if [[ -z "$_E2E_IBOX_USERNAME" ]]; then
    echo "Must provide _E2E_IBOX_USERNAME in environment" 1>&2
    exit 1
fi
if [[ -z "$_E2E_IBOX_PASSWORD" ]]; then
    echo "Must provide _E2E_IBOX_PASSWORD in environment" 1>&2
    exit 1
fi
if [[ -z "$_E2E_NAMESPACE" ]]; then
    echo "Must provide _E2E_NAMESPACE in environment" 1>&2
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
if [[ -z "$_E2E_PROTOCOL" ]]; then
    echo "Must provide _E2E_PROTOCOL in environment" 1>&2
    exit 1
fi

echo 'running csi-sanity tests'
# create working dir
export WORKDIR=`mktemp -d /tmp/csi-sanity.XXXXXX`
echo $WORKDIR

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
echo $SCRIPT_DIR

rm -rf /tmp/csi-mount /tmp/csi-staging
export KUBEHOST=`go run $SCRIPT_DIR/gethost.go`
echo -e $KUBEHOST was the kubehost

# copy test manifests to working dir
envsubst < $SCRIPT_DIR/csi-secrets.yaml > $WORKDIR/csi-secrets.yaml
envsubst < $SCRIPT_DIR/test-volume-parameters.yaml > $WORKDIR/test-volume-parameters.yaml
envsubst < $SCRIPT_DIR/test-snapshot-parameters.yaml > $WORKDIR/test-snapshot-parameters.yaml

# example flag to run a single test
#        -ginkgo.focus "should\ fail\ when\ volume\ is\ not\ found"  \

# example flags to skip certain tests
#        -ginkgo.skip "maximum\-length"  \
#        -ginkgo.skip "length"  \
#        -ginkgo.skip "should\ return\ empty\ when\ the\ specified\ snapshot\ id\ does\ not\ exist"  \

csi-sanity -ginkgo.v \
        -ginkgo.skip "maximum"  \
        -ginkgo.skip "length"  \
        -ginkgo.skip "Node"  \
	-csi.endpoint $KUBEHOST:30008  \
	-csi.controllerendpoint $KUBEHOST:30007  \
	-csi.testvolumeparameters $WORKDIR/test-volume-parameters.yaml \
	-csi.testsnapshotparameters $WORKDIR/test-snapshot-parameters.yaml \
	-csi.secrets $WORKDIR/csi-secrets.yaml &> $WORKDIR/results.log
