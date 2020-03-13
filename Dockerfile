FROM ubuntu:18.04

COPY setenv.sh /setenv.sh
RUN chmod +x /setenv.sh
COPY infinibox-csi-driver /infinibox-csi-driver

RUN mkdir /ibox
ADD host-chroot.sh /ibox
RUN chmod 777 /ibox/host-chroot.sh
RUN    ln -s /ibox/host-chroot.sh /ibox/blkid \
    && ln -s /ibox/host-chroot.sh /ibox/blockdev \
    && ln -s /ibox/host-chroot.sh /ibox/iscsiadm \
    && ln -s /ibox/host-chroot.sh /ibox/rpcbind \
    && ln -s /ibox/host-chroot.sh /ibox/lsblk \
    && ln -s /ibox/host-chroot.sh /ibox/lsscsi \
    && ln -s /ibox/host-chroot.sh /ibox/mkfs.ext3 \
    && ln -s /ibox/host-chroot.sh /ibox/mkfs.ext4 \
    && ln -s /ibox/host-chroot.sh /ibox/mkfs.xfs \
    && ln -s /ibox/host-chroot.sh /ibox/fsck \
    && ln -s /ibox/host-chroot.sh /ibox/mount \
    && ln -s /ibox/host-chroot.sh /ibox/multipath \
    && ln -s /ibox/host-chroot.sh /ibox/multipathd \
    && ln -s /ibox/host-chroot.sh /ibox/cat \
    && ln -s /ibox/host-chroot.sh /ibox/mkdir \
    && ln -s /ibox/host-chroot.sh /ibox/rmdir \
    && ln -s /ibox/host-chroot.sh /ibox/umount
COPY multipath.conf /etc/multipath.conf

ENV PATH="/ibox:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"


ENTRYPOINT ["/setenv.sh"]