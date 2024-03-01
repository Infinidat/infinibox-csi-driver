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
	#-ginkgo.focus "should\ return\ snapshots\ that\ match\ the\ specified\ source\ volume\ id" \

# example flags to skip certain tests
#        -ginkgo.skip "maximum\-length"  \
#        -ginkgo.skip "length"  \
#        -ginkgo.skip "should\ return\ empty\ when\ the\ specified\ snapshot\ id\ does\ not\ exist"  \

# skipped tests and why we skip them
#	-ginkgo.focus "check\ the\ presence\ of\ new\ volumes\ and\ absence\ of\ deleted\ ones\ in\ the\ volume\ list" \
# this test doesn't create PVCs like normal, intead it calls CreateVolume() directly, so our logic of finding and counting PVs 
# created by the CSI driver doesn't work.  We could change our ListVolumes() implementation to look at the datasets on the Ibox
# and return those values, but our current logic depends on collecting the storageclass information for PVs and returning
# that to the end user, so to fix this would require a new ListVolumes() implementation
#
#	-ginkgo.focus "should\ return\ next\ token\ when\ a\ limited\ number\ of\ entries\ are\ requested" \
# this test requires pagination in the ListSnapshots() to be implemented, the current implementation does
# not support the MaxEntries request parameter and thus paginating results, all results are returned regardless.

csi-sanity -ginkgo.v \
	-ginkgo.skip "should\ return\ next\ token\ when\ a\ limited\ number\ of\ entries\ are\ requested" \
	-ginkgo.skip "check\ the\ presence\ of\ new\ volumes\ and\ absence\ of\ deleted\ ones\ in\ the\ volume\ list" \
        -ginkgo.skip "maximum"  \
        -ginkgo.skip "length"  \
        -ginkgo.skip "Node"  \
	-csi.endpoint $KUBEHOST:30008  \
	-csi.controllerendpoint $KUBEHOST:30007  \
	-csi.testvolumeparameters $WORKDIR/test-volume-parameters.yaml \
	-csi.testsnapshotparameters $WORKDIR/test-snapshot-parameters.yaml \
	-csi.secrets $WORKDIR/csi-secrets.yaml &> $WORKDIR/results.log
