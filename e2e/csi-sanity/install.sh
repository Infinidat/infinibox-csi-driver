#!/bin/bash
echo 'Installing CSI sanity test binary...'
WORKDIR=/tmp/kubernetes-csi
rm -rf $WORKDIR
mkdir -p $WORKDIR
pushd $WORKDIR
git clone https://github.com/kubernetes-csi/csi-test.git
cd $WORKDIR/csi-test
git checkout v5.2.0
cd $WORKDIR/csi-test/cmd/csi-sanity
make
pwd
sudo cp csi-sanity /usr/local/bin/
