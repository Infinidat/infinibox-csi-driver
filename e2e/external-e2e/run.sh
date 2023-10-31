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

echo $_E2E_NAMESPACE is e2e namespace

echo $_E2E_PROTOCOL is the protocol we are testing

echo 'running external e2e.test ...'
# create working dir
export WORKDIR=`mktemp -d /tmp/external-e2e.$_E2E_PROTOCOL.XXXXXX`
echo $WORKDIR

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
echo $SCRIPT_DIR

# copy test manifests to working dir
envsubst < $SCRIPT_DIR/e2e-manifest-$_E2E_PROTOCOL.yaml > $WORKDIR/e2e-manifest.yaml
envsubst < $SCRIPT_DIR/e2e-storageclass-$_E2E_PROTOCOL.yaml > $WORKDIR/e2e-storageclass-$_E2E_PROTOCOL.yaml
envsubst < $SCRIPT_DIR/e2e-volume-snapshotclass.yaml > $WORKDIR/e2e-volume-snapshotclass.yaml

# run test
#	-ginkgo.v \
#	-ginkgo.dry-run \
# -ginkgo.focus='External.Storage' \
# -ginkgo.focus='External.Storage.*Dynamic*'  \
# -ginkgo.focus='External.Storage.*infinibox-csi-driver'  \
#	--ginkgo.skip="disruptive" \
#	--ginkgo.skip="Disruptive" \
#	--ginkgo.skip="ephemeral" \
./e2e.test -ginkgo.focus='External.Storage.*infinibox-csi-driver' \
	-ginkgo.progress \
	--ginkgo.skip="disruptive" \
	--ginkgo.skip="Disruptive" \
	--ginkgo.skip="ephemeral" \
	--ginkgo.skip="Ephemeral" \
	--ginkgo.skip="access to two volumes" \
	--ginkgo.skip="when restoring snapshot to larger size pvc" \
	-storage.testdriver=$WORKDIR/e2e-manifest.yaml \
	> $WORKDIR/results.log
