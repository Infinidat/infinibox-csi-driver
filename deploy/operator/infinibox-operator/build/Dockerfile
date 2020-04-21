FROM quay.io/operator-framework/helm-operator:v0.15.0

MAINTAINER partners.infi@infinidat.com 
LABEL name="infinidat-csi-operator" \
      vendor="Infinidat" \
      summary="Infinidat CSI Operator" \
      description="CSI operator will install infinibox csi driver"  

COPY licenses /licenses

COPY watches.yaml ${HOME}/watches.yaml
COPY helm-charts/ ${HOME}/helm-charts/
