#!/bin/sh
echo 'Installing redhat e2e tests'
BRANCH=release-4.11
WORKDIR=/tmp/redhat-e2e
rm -rf $WORKDIR
mkdir -p $WORKDIR
pushd $WORKDIR
git clone https://github.com/openshift/origin.git $BRANCH
ce $BRANCH
make
