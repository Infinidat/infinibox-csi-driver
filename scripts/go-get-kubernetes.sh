#!/bin/sh
#
# This is a simpler form of the go-get-kubernetes.sh script, in function - it solves the same issue
# in a more concise fashion. It does not address "packages."
#
# Script obtained from: https://github.com/kubernetes/kubernetes/issues/79384#issuecomment-521493597
#
# The original script this replaced, can be found here: 
#    https://raw.githubusercontent.com/kubernetes-csi/csi-release-tools/master/go-get-kubernetes.sh
# Users should refer to that should this script ever present a problem. 
#
set -euo pipefail

VERSION=${1#"v"}
if [ -z "$VERSION" ]; then
    echo "Must specify version!"
    exit 1
fi
MODS=($(
    curl -sS https://raw.githubusercontent.com/kubernetes/kubernetes/v${VERSION}/go.mod |
    sed -n 's|.*k8s.io/\(.*\) => ./staging/src/k8s.io/.*|k8s.io/\1|p'
))
for MOD in "${MODS[@]}"; do
    echo -n "."
    V=$(
        go mod download -json "${MOD}@kubernetes-${VERSION}" |
        sed -n 's|.*"Version": "\(.*\)".*|\1|p'
    )
    go mod edit "-replace=${MOD}=${MOD}@${V}"
done
echo "."
go get "k8s.io/kubernetes@v${VERSION}"
go mod tidy -v 