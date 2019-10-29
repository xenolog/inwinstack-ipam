ARG FROM=docker-dev-local.docker.mirantis.net/mirantis/general/ubuntu/bionic:base-bionic-20191028014513
FROM $FROM

COPY out/inwinstack-ipam /bin

LABEL io.k8s.display-name="inwinstack-ipam (Mirantis build)" \
      io.k8s.description="This is the image of IP/pool storage for the KaaS BareMetal IPAM"

ENTRYPOINT ["inwinstack-ipam"]
