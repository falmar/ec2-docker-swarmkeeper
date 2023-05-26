package manager

// --
// TODO: implement raft quorum for:
// 1. leader election, so one service will long poll for events and take action
// --

// --
// This manager command will be responsible for
// 1. draining worker nodes
// 2. removing them from the swarm cluster
// 3. terminating (complete lifecycle hook) the EC2 instance
// --
