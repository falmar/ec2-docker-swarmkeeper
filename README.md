# EC2 Docker Swarm[keeper]

**DISCLAIMER: Please don't use this!, I'm not responsible for any damage this may cause.**

*Why does this exist? the weekend was really boring, so here we are creating tools for the sake of learning...* it exists for anyone looking
for my silly Golang projects.

This is a personal project that is meant to be a daemon on docker swarm, its primary goal is clean up worker nodes that
go away due to Spot terminations, Spot Autoscaling Re-balance or Autoscaling Lifecycle updates such as scale-in/out or
template changes.

Worker nodes get drained so services can move to other available nodes in the cluster, the node remain as part of the
cluster while it get deregistered from load balancers or enters autoscaling lifecycles.

*It works for my use case on production.* lol! (under 10 nodes), and I will continue to use it and add more features as I need them. 

TODO:

- add a proper README.md
- explain the AWS services used, permissions, what and why.
