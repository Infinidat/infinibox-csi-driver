#FROM ubuntu:18.04
#FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
#FROM redhat/ubi8-minimal:8.4-205
FROM redhat/ubi8:8.4-206.1626828523

# Note: See "linux_host" note the IBox Ansible vars yaml
# file in git.infinidat.com:PSUS/webinar-automate-sla.git.
# This is affected by the base image choice.

MAINTAINER partners.infi@infinidat.com

LABEL name="infinibox-csi-driver"
LABEL vendor="Infinidat"
LABEL summary="Infinidat CSI-Plugin"
LABEL description="A CSI Driver image for InfiniBox"
LABEL org.opencontainers.image.authors="partners.infi@infinidat.com"

COPY licenses /licenses
COPY setenv.sh /setenv.sh
RUN chmod +x /setenv.sh
COPY infinibox-csi-driver /infinibox-csi-driver
RUN chmod +x /infinibox-csi-driver

RUN yum -y install file lsof && \
    yum -y clean all && rm -rf /var/cache

RUN mkdir /ibox
ADD host-chroot.sh /ibox
RUN chmod 777 /ibox/host-chroot.sh
RUN \
       ln -s /ibox/host-chroot.sh /ibox/blkid \
    && ln -s /ibox/host-chroot.sh /ibox/blockdev \
    && ln -s /ibox/host-chroot.sh /ibox/cat \
    && ln -s /ibox/host-chroot.sh /ibox/chown \
    && ln -s /ibox/host-chroot.sh /ibox/chmod \
    && ln -s /ibox/host-chroot.sh /ibox/dmsetup \
    && ln -s /ibox/host-chroot.sh /ibox/file \
    && ln -s /ibox/host-chroot.sh /ibox/find \
    && ln -s /ibox/host-chroot.sh /ibox/fsck \
    && ln -s /ibox/host-chroot.sh /ibox/hostnamectl \
    && ln -s /ibox/host-chroot.sh /ibox/iscsiadm \
    && ln -s /ibox/host-chroot.sh /ibox/lsblk \
    && ln -s /ibox/host-chroot.sh /ibox/lsof \
    && ln -s /ibox/host-chroot.sh /ibox/lsscsi \
    && ln -s /ibox/host-chroot.sh /ibox/mkdir \
    && ln -s /ibox/host-chroot.sh /ibox/mkfs.ext3 \
    && ln -s /ibox/host-chroot.sh /ibox/mkfs.ext4 \
    && ln -s /ibox/host-chroot.sh /ibox/mkfs.xfs \
    && ln -s /ibox/host-chroot.sh /ibox/mount \
    && ln -s /ibox/host-chroot.sh /ibox/multipath \
    && ln -s /ibox/host-chroot.sh /ibox/multipathd \
    && ln -s /ibox/host-chroot.sh /ibox/rmdir \
    && ln -s /ibox/host-chroot.sh /ibox/rpcbind \
    && ln -s /ibox/host-chroot.sh /ibox/umount \
    && ln -s /ibox/host-chroot.sh /ibox/vi \
    && ln -s /ibox/host-chroot.sh /ibox/vim \
    && ln -s /ibox/host-chroot.sh /ibox/whoami \
    && ln -s /ibox/host-chroot.sh /ibox/xfs_admin \
    && ln -s /ibox/host-chroot.sh /ibox/xfs_db

ENV PATH="/ibox:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"


ENTRYPOINT ["/setenv.sh"]
