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
#	--ginkgo.skip="disruptive" \
#	--ginkgo.skip="Disruptive" \
#	--ginkgo.skip="ephemeral" \
#	--ginkgo.skip="Ephemeral" \
#	--ginkgo.skip="access to two volumes" \
#	--ginkgo.skip="OnRootMismatch" \
#	--ginkgo.skip="when restoring snapshot to larger size pvc" \
./e2e.test  \
	-ginkgo.no-color \
	-storage.testdriver=$WORKDIR/e2e-manifest.yaml \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ file\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ readOnly\ file\ specified\ in\ the\ volumeMount\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ restarting\ containers\ using\ file\ as\ subpath\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ read-write-once-pod\ should\ block\ a\ second\ pod\ from\ using\ an\ in-use\ ReadWriteOncePod\ volume\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ readOnly\ file\ specified\ in\ the\ volumeMount\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\(allowExpansion\)\]\ volume-expand\ Verify\ if\ offline\ PVC\ expansion\ works' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ with\ backstepping\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ with\ backstepping\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ Snapshot\ \(delete\ policy\)\]\ snapshottable\[Feature\:VolumeSnapshotDataSource\]\ volume\ snapshot\ controller\ \ should\ check\ snapshot\ fields,\ check\ restore\ correctly\ works\ after\ modifying\ source\ data,\ check\ deletion\ \(persistent\)' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(ext3\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ file\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ volumeLimits\ should\ support\ volume\ limits\ \[Serial\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ verify\ container\ cannot\ write\ to\ subpath\ readonly\ volumes\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ volumeMode\ should\ fail\ to\ use\ a\ volume\ in\ a\ pod\ with\ mismatched\ mode\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext3\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ restarting\ containers\ using\ directory\ as\ subpath\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ creating\ multiple\ subpath\ from\ same\ volumes\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ single\ file\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(xfs\)\]\[Slow\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ restarting\ containers\ using\ directory\ as\ subpath\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ restored\ snapshot\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSnapshotDataSource\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ file\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ non-existent\ path' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(xfs\)\]\[Slow\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ restored\ snapshot\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSnapshotDataSource\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ restarting\ containers\ using\ file\ as\ subpath\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(xfs\)\]\[Slow\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ directories\ when\ readOnly\ specified\ in\ the\ volumeSource' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ non-existent\ subpath\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext4\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ creating\ multiple\ subpath\ from\ same\ volumes\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ its\ clone\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ Snapshot\ \(retain\ policy\)\]\ snapshottable-stress\[Feature\:VolumeSnapshotDataSource\]\ should\ support\ snapshotting\ of\ many\ volumes\ repeatedly\ \[Slow\]\ \[Serial\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ non-existent\ path' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ provisioning\ should\ mount\ multiple\ PV\ pointing\ to\ the\ same\ storage\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ directory\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ non-existent\ subpath\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ restarting\ containers\ using\ directory\ as\ subpath\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ directory\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ directory\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ non-existent\ path' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext4\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ read-only\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ read-only\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\(allowExpansion\)\]\ volume-expand\ Verify\ if\ offline\ PVC\ expansion\ works' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ non-existent\ subpath\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(xfs\)\]\[Slow\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(xfs\)\]\[Slow\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ its\ clone\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ volume-stress\ multiple\ pods\ should\ access\ different\ volumes\ repeatedly\ \[Slow\]\ \[Serial\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ fail\ if\ subpath\ with\ backstepping\ is\ outside\ the\ volume\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ directory' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext4\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ volume-expand\ should\ not\ allow\ expansion\ of\ pvcs\ without\ AllowVolumeExpansion\ property' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ volumeLimits\ should\ verify\ that\ all\ csinodes\ have\ volume\ limits' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ provisioning\ should\ provision\ storage\ with\ mount\ options' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(delayed\ binding\)\]\ topology\ should\ provision\ a\ volume\ and\ schedule\ a\ pod\ with\ AllowedTopologies' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ fsgroupchangepolicy\ \(Always\)\[LinuxOnly\],\ pod\ created\ with\ an\ initial\ fsgroup,\ volume\ contents\ ownership\ changed\ via\ chgrp\ in\ first\ pod,\ new\ pod\ with\ same\ fsgroup\ applied\ to\ the\ volume\ contents' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ restored\ snapshot\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSnapshotDataSource\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ restored\ snapshot\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSnapshotDataSource\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ readOnly\ directory\ specified\ in\ the\ volumeMount' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext4\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ its\ clone\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ provisioning\ should\ provision\ storage\ with\ pvc\ data\ source' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ fsgroupchangepolicy\ \(Always\)\[LinuxOnly\],\ pod\ created\ with\ an\ initial\ fsgroup,\ volume\ contents\ ownership\ changed\ via\ chgrp\ in\ first\ pod,\ new\ pod\ with\ different\ fsgroup\ applied\ to\ the\ volume\ contents' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(immediate\ binding\)\]\ topology\ should\ provision\ a\ volume\ and\ schedule\ a\ pod\ with\ AllowedTopologies' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ file\ as\ subpath\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext4\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ restored\ snapshot\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSnapshotDataSource\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(xfs\)\]\[Slow\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ read-only\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ provisioning\ should\ provision\ storage\ with\ snapshot\ data\ source\ \[Feature\:VolumeSnapshotDataSource\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ provisioning\ should\ provision\ storage\ with\ pvc\ data\ source\ in\ parallel\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ read-only\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(immediate\ binding\)\]\ topology\ should\ fail\ to\ schedule\ a\ pod\ which\ has\ topologies\ that\ conflict\ with\ AllowedTopologies' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ provisioning\ should\ provision\ storage\ with\ any\ volume\ data\ source\ \[Serial\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ directory' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(xfs\)\]\[Slow\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ different\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ read-only\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ read-only\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ different\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ provisioning\ should\ provision\ storage\ with\ pvc\ data\ source' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ provisioning\ should\ provision\ storage\ with\ pvc\ data\ source\ in\ parallel\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ volumeMode\ should\ not\ mount\ /\ map\ unused\ volumes\ in\ a\ pod\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ provisioning\ should\ provision\ storage\ with\ mount\ options' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ read-write-once-pod\ should\ preempt\ lower\ priority\ pods\ using\ ReadWriteOncePod\ volumes' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(ext4\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(ext4\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext4\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ different\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ verify\ container\ cannot\ write\ to\ subpath\ readonly\ volumes\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ volumeMode\ should\ fail\ to\ use\ a\ volume\ in\ a\ pod\ with\ mismatched\ mode\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\(allowExpansion\)\]\ volume-expand\ should\ resize\ volume\ when\ PVC\ is\ edited\ while\ pod\ is\ using\ it' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ readOnly\ file\ specified\ in\ the\ volumeMount\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ provisioning\ should\ mount\ multiple\ PV\ pointing\ to\ the\ same\ storage\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ readOnly\ directory\ specified\ in\ the\ volumeMount' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(block\ volmode\)\]\ volumeMode\ should\ not\ mount\ /\ map\ unused\ volumes\ in\ a\ pod\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(block\ volmode\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ different\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ capacity\ provides\ storage\ capacity\ information' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(ext3\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ Snapshot\ \(retain\ policy\)\]\ snapshottable\[Feature\:VolumeSnapshotDataSource\]\ volume\ snapshot\ controller\ \ should\ check\ snapshot\ fields,\ check\ restore\ correctly\ works\ after\ modifying\ source\ data,\ check\ deletion\ \(persistent\)' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(xfs\)\]\[Slow\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(ext3\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ volume-lifecycle-performance\ should\ provision\ volumes\ at\ scale\ within\ performance\ constraints\ \[Slow\]\ \[Serial\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ file\ as\ subpath\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ provisioning\ should\ provision\ storage\ with\ any\ volume\ data\ source\ \[Serial\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ Snapshot\ \(retain\ policy\)\]\ snapshottable\[Feature\:VolumeSnapshotDataSource\]\ volume\ snapshot\ controller\ \ should\ check\ snapshot\ fields,\ check\ restore\ correctly\ works\ after\ modifying\ source\ data,\ check\ deletion\ \(persistent\)' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ readOnly\ directory\ specified\ in\ the\ volumeMount' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(filesystem\ volmode\)\]\ volumeMode\ should\ not\ mount\ /\ map\ unused\ volumes\ in\ a\ pod\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ Snapshot\ \(delete\ policy\)\]\ snapshottable-stress\[Feature\:VolumeSnapshotDataSource\]\ should\ support\ snapshotting\ of\ many\ volumes\ repeatedly\ \[Slow\]\ \[Serial\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(delayed\ binding\)\]\ topology\ should\ fail\ to\ schedule\ a\ pod\ which\ has\ topologies\ that\ conflict\ with\ AllowedTopologies' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ file\ as\ subpath\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ restarting\ containers\ using\ file\ as\ subpath\ \[Slow\]\[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ directories\ when\ readOnly\ specified\ in\ the\ volumeSource' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ restored\ snapshot\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSnapshotDataSource\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(xfs\)\]\[Slow\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ the\ same\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ volumeIO\ should\ write\ files\ of\ various\ sizes,\ verify\ size,\ validate\ content\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ single\ file\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ volumeIO\ should\ write\ files\ of\ various\ sizes,\ verify\ size,\ validate\ content\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ verify\ container\ cannot\ write\ to\ subpath\ readonly\ volumes\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ be\ able\ to\ unmount\ after\ the\ subpath\ directory\ is\ deleted\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ subPath\ should\ be\ able\ to\ unmount\ after\ the\ subpath\ directory\ is\ deleted\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ directory' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ directories\ when\ readOnly\ specified\ in\ the\ volumeSource' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(filesystem\ volmode\)\]\ volumeMode\ should\ not\ mount\ /\ map\ unused\ volumes\ in\ a\ pod\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ Snapshot\ \(delete\ policy\)\]\ snapshottable\[Feature\:VolumeSnapshotDataSource\]\ volume\ snapshot\ controller\ \ should\ check\ snapshot\ fields,\ check\ restore\ correctly\ works\ after\ modifying\ source\ data,\ check\ deletion\ \(persistent\)' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ volumeIO\ should\ write\ files\ of\ various\ sizes,\ verify\ size,\ validate\ content\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ existing\ single\ file\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ subPath\ should\ support\ creating\ multiple\ subpath\ from\ same\ volumes\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext3\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(xfs\)\]\[Slow\]\ volumes\ should\ store\ data' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(xfs\)\]\[Slow\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(ext4\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ its\ clone\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(block\ volmode\)\]\ volumeMode\ should\ fail\ to\ use\ a\ volume\ in\ a\ pod\ with\ mismatched\ mode\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ volume-stress\ multiple\ pods\ should\ access\ different\ volumes\ repeatedly\ \[Slow\]\ \[Serial\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(default\ fs\)\]\ subPath\ should\ be\ able\ to\ unmount\ after\ the\ subpath\ directory\ is\ deleted\ \[LinuxOnly\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ its\ clone\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(ext4\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\(allowExpansion\)\]\ volume-expand\ should\ resize\ volume\ when\ PVC\ is\ edited\ while\ pod\ is\ using\ it' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ provisioning\ should\ provision\ storage\ with\ snapshot\ data\ source\ \[Feature\:VolumeSnapshotDataSource\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(filesystem\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ volume\ and\ its\ clone\ from\ pods\ on\ the\ same\ node\ \[LinuxOnly\]\[Feature\:VolumeSourceXFS\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ volume-expand\ should\ not\ allow\ expansion\ of\ pvcs\ without\ AllowVolumeExpansion\ property' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ different\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Inline-volume\ \(default\ fs\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(block\ volmode\)\]\ multiVolume\ \[Slow\]\ should\ concurrently\ access\ the\ single\ volume\ from\ pods\ on\ different\ node' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Dynamic\ PV\ \(default\ fs\)\]\ fsgroupchangepolicy\ \(Always\)\[LinuxOnly\],\ pod\ created\ with\ an\ initial\ fsgroup,\ new\ pod\ fsgroup\ applied\ to\ volume\ contents' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(filesystem\ volmode\)\]\ volumeMode\ should\ fail\ to\ use\ a\ volume\ in\ a\ pod\ with\ mismatched\ mode\ \[Slow\]' \
	--ginkgo.focus 'External\ Storage\ \[Driver\:\ infinibox-csi-driver\]\ \[Testpattern\:\ Pre-provisioned\ PV\ \(ext4\)\]\ volumes\ should\ allow\ exec\ of\ files\ on\ the\ volume' \
	--storage.testdriver=$WORKDIR/e2e-manifest.yaml \
	> $WORKDIR/results.log
