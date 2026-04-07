#!/bin/bash
# $1 - the remote hostname to setup (e.g. csidev-ocp421)

if test -z "$1"; then
	echo "The remote hostname variable is empty and is required"
	echo "example: " $0 csidev-ocp421
	exit 1
fi

echo "going to set up remote test environment on" $1
REMOTE=$1
SSH="ssh -i ~/.ssh/ocp_id_rsa core@"$REMOTE
SCP="scp -i ~/.ssh/ocp_id_rsa "
$SSH pwd

# ssh keys are required to run 'git clone'
cd ~ && tar czf /tmp/ssh.tar.gz .ssh
$SCP /tmp/ssh.tar.gz core@$1:/tmp/
$SSH -C "cd ~ && tar xzf /tmp/ssh.tar.gz"

# .docker is copied over, not used currently
cd ~ && tar czf /tmp/docker.tar.gz .docker
$SCP /tmp/docker.tar.gz core@$1:/tmp/
$SSH -C "cd ~ && tar xzf /tmp/docker.tar.gz"

# set up a bin directory to hold various required commands
$SSH mkdir bin
$SCP /usr/bin/make core@$1:~/bin

# copy over a working bashrc and git-completion
$SCP  ~/.bashrc  core@$1:
$SCP  ~/.git-com*  core@$1:

# copy over the working k8screds we'll use to log into k8s
$SCP -r ~/k8screds  core@$1:

#$SCP -r ~/.git  core@$1:

# setup golang on the remote
$SCP  ~/Downloads/go1*.tar.gz  core@$1:
$SSH "tar xvzf ~/go*.tar.gz"

# clone the develop branch on the remote
$SSH -C "git clone git@git.infinidat.com:jmccormick/infinidat-csi-driver.git"

# copy a working copy of the Makefile-vars-git-ignored file to use
$SCP  ~/infinidat-csi-driver/Makefile-vars-git-ignored  core@$1:/tmp
$SSH -C "cp /tmp/Makefile-vars-git-ignored ~/infinidat-csi-driver/"

