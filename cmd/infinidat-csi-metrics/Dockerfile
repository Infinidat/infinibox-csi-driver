FROM redhat/ubi8:latest

# Note: See "linux_host" note the IBox Ansible vars yaml
# file in git.infinidat.com:PSUS/webinar-automate-sla.git.
# This is affected by the base image choice.
# Base image is specified in Makefile and must match.

MAINTAINER partners.infi@infinidat.com

ARG   BLAME_MACHINE
ARG   BLAME_USER
ARG   BLAME_BUILD_TIME
ARG   IMAGE_TAG
ARG   VCS_REF
ENV   BLAME_MACHINE=$BLAME_MACHINE
ENV   BLAME_USER=$BLAME_USER
ENV   BLAME_BUILD_TIME=$BLAME_BUILD_TIME
ENV   IMAGE_TAG=$IMAGE_TAG
ENV   VCS_REF=$VCS_REF
LABEL BLAME_MACHINE=$BLAME_MACHINE
LABEL BLAME_USER=$BLAME_USER
LABEL BLAME_BUILD_TIME=$BLAME_BUILD_TIME
LABEL IMAGE_TAG=$IMAGE_TAG
LABEL VCS_REF=$VCS_REF
LABEL description="A CSI metrics image for InfiniBox"
LABEL name="infinibox-csi-metrics"
LABEL org.opencontainers.image.authors="partners.infi@infinidat.com"
LABEL summary="Infinidat CSI-Plugin Metrics exporter"
LABEL vendor="Infinidat"

COPY licenses /licenses
COPY infinibox-csi-metrics /infinibox-csi-metrics
RUN chmod +x /infinibox-csi-metrics

RUN yum -y install file lsof hostname && \
	yum -y update && \
    yum -y clean all && rm -rf /var/cache

ENTRYPOINT ["/infinibox-csi-metrics"]
