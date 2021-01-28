#!/usr/bin/env bash

# CSI node daemonset mounts / of the host into /host inside the container.
# This tool chroots into $DIR and runs the command.  The chroot exists
# only the for the life of the command execution.

declare -r COMMAND="$(basename "$0")"
declare -r DIR="/host"

if [ ! -d "${DIR}" ]; then
    echo "Could not find docker engine host's filesystem at expected location: ${DIR}"
    exit 1
fi

# chroot a single command
exec chroot $DIR /usr/bin/env --ignore-environment PATH="/sbin:/bin:/usr/bin" "${COMMAND}" "${@:1}"
