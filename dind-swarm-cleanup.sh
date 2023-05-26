#!/bin/bash

# remove containers
sudo docker container rm -f manager worker-1 worker-2 worker-3

sudo docker network rm swarm

sudo docker volume rm some-docker-certs-ca some-docker-certs-client
