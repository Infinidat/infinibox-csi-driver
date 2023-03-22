#!/bin/sh
echo 'Installing external e2e.test binary...'
WORKDIR=/tmp/external-e2e
rm -rf $WORKDIR
mkdir -p $WORKDIR
pushd $WORKDIR
curl -sL https://storage.googleapis.com/kubernetes-release/release/v1.24.0/kubernetes-test-linux-amd64.tar.gz --output e2e-tests.tar.gz
tar -xvf e2e-tests.tar.gz && rm e2e-tests.tar.gz
sudo cp $WORKDIR/kubernetes/test/bin/e2e.test /usr/local/bin/
sudo cp $WORKDIR/kubernetes/test/bin/ginkgo /usr/local/bin/
