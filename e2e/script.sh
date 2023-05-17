#!/bin/bash

function sigterm() {
    echo "Got SIGTERM"
    exit
}

trap sigterm SIGTERM

i=1
while true; do
    echo "$(date +%H:%M:%S) | $((i++)) | $HOSTNAME" >> /tmp/data/status
    ls -l /tmp/data/status
    sleep 10
done
