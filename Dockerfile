FROM centos:7.4.1708
# RUN yum install -y libaio
# RUN yum install -y libuuid
# RUN yum install -y numactl
# RUN yum install -y xfsprogs
# RUN yum install -y e4fsprogs
RUN yum -y install iscsi-initiator-utils e2fsprogs xfsprogs && yum clean all
RUN yum -y install nfs-utils 
#cifs-utils
# original
# COPY "infinibox-csi-driver" .
# ENTRYPOINT ["./infinibox-csi-driver"]
 

# my trial and error 1
#COPY infinibox-csi-driver .
#RUN echo "InitiatorName=${ISCSI_INITIATOR_NAME}" > /etc/iscsi/initiatorname.iscsi 
# Start iscsid
#RUN	iscsid -f &
#ENTRYPOINT ["./infinibox-csi-driver"]
# my trial and error 1

COPY setenv.sh /setenv.sh
RUN chmod +x /setenv.sh
COPY infinibox-csi-driver /infinibox-csi-driver
ENTRYPOINT ["/setenv.sh"]

#RUN ./setenv.sh
