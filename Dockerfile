FROM ubuntu:18.04
RUN apt-get update && apt-get install -y apt-transport-https
#RUN apt-get update && apt-get install -y kmod
RUN apt-get update && apt-get install -y open-iscsi
RUN apt-get update && apt-get install -y nfs-common
RUN apt-get update && apt-get install -y xfsprogs 

COPY setenv.sh /setenv.sh
RUN chmod +x /setenv.sh
COPY infinibox-csi-driver /infinibox-csi-driver
ENTRYPOINT ["/setenv.sh"]