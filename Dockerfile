FROM centos:centos6

COPY dockerMan /bin/dockerMan
EXPOSE 8080
CMD dockerMan
