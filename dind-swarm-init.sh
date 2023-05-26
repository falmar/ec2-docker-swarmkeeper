#!/bin/bash

# AWS like
sudo docker network create --driver=bridge swarm --subnet=172.28.28.0/24

ip_base="172.28.28."
counter=2

for node in manager worker-1 worker-2 worker-3; do
  ip_address=$ip_base$counter
  echo "Creating $node with IP address $ip_address"

  sudo docker run --privileged --name $node -d \
    --network swarm --network-alias $node \
    --ip $ip_address \
    -e DOCKER_TLS_CERTDIR=/certs \
    -v some-docker-certs-ca:/certs/ca \
    -v some-docker-certs-client:/certs/client \
    $(if [ "$node" == "manager" ]; then echo "-p 2365:2375 -p 2366:2376 -p 2367:2377"; fi) \
    docker:dind

  counter=$((counter + 1))
done
# wait for containers to start
echo "Waiting 5s for containers to start..."
sleep 5

# get join token and add workers to swarm
sudo docker exec manager docker swarm init --advertise-addr 172.28.28.2:2377

# get join token
token=$(sudo docker exec manager docker swarm join-token worker -q)

# add workers to swarm
for node in worker-1 worker-2 worker-3; do
  sudo docker exec -it $node docker swarm join --token $token manager:2377
done
