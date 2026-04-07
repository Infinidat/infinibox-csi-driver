#!/bin/bash
# $1 - the remote hostname name (e.g. csidev-ocp421)
# $2 - the Makefile test target to run
if test -z "$1"; then
    echo "The remote hostname variable is empty and is required."
    echo e.g. $0 csidev-ocp421 e2enfs
    exit 1
fi
if test -z "$2"; then
    echo "The Makefile test target is empty and is required."
    echo e.g. $0 csidev-ocp421 e2enfs
    exit 1
fi

echo "going to run remote test $2 on environment " $1
REMOTE=$1
TESTSUITE=$2
SSH="ssh -i ~/.ssh/ocp_id_rsa core@"$REMOTE
SCP="scp -i ~/.ssh/ocp_id_rsa "
$SSH -C "kfig $REMOTE && cd infinidat-csi-driver && make $TESTSUITE > /tmp/$REMOTE-$TESTSUITE.out"
$SCP core@$REMOTE:/tmp/$REMOTE-$TESTSUITE.out /tmp
cat /tmp/$REMOTE-$TESTSUITE.out
