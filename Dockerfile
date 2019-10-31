ARG FROM=docker-dev-local.docker.mirantis.net/mirantis/general/ubuntu/bionic:base-bionic-20191028014513
FROM $FROM

COPY out/inwinstack-ipam /bin

RUN apt-get update && apt-get install -y \
    musl \
 && rm -rf /var/lib/apt/lists/*

LABEL io.k8s.display-name="inwinstack-ipam (Mirantis build)" \
      io.k8s.description="This is the image of IP/pool storage for Mirantis' KaaS BareMetal IPAM"

ENTRYPOINT ["inwinstack-ipam"]
