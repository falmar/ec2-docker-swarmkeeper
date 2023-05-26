#!/bin/bash

# remove containers
sudo docker container rm -f manager worker-1 worker-2 worker-3
# remove network
sudo docker network rm swarm
# remove volumes
sudo docker volume rm some-docker-certs-ca some-docker-certs-client
