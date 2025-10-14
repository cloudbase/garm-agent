# GARM Agent

GARM agent is a small service that enhances the interaction between a Git(Hub|Tea) runner instance and a GARM server.

## Rationale

Keeping state in sync between a runner auto scaler, a Forge (github, gitea, etc) and a potential IaaS provider in which we need ti spin up the actual compute instances that become the runners, can become quite complex and brittle.

The state in GARM is completely under our control. Anything that happens within GARM itself is somethign we can easily see. A runner, however is also represented within the Forge itself. You can see it in your repo/org/enterprise settings. But that by itself is only one third of the state, because we also have the compute instance which is represented in the IaaS provider.

Either one of these three can be influenced by external factors. A runner can be deleted from the Forge, a compute instance can be terminated by the user or some automated process, etc.

To keep all this in sync, we need to periodically list all runners within an entity (repo/org/enterprise) and compare that to the runners we have in GARM. But just because we have a runner in GARM and in the Forge, does not mean that the compute instance is there. Conversely, we might have a compute instance running, but the runner is gone from the Forge (job was completed, but the webhook never arived).

So we need to handle

* Runners that are orphaned in the Forge
* Runners that are orphaned in GARM
* Runners that are orphaned in the IaaS provider

All of this generates a lot of API requests, which are subject to various rate limits and technical limitations. Some APIs are paginated, which is not necessarily an issue in itself, but in very large orgs/enterprises, this can lead to situations in which, as we finish the request on page N, a runner is removed from page N-1 and the whole list shifts towards the beginning, moving some runners from page N+1 to page N (which we just finished listing). So we miss some runners from page N+1 once we request that, leading to GARM thinking that those runners are orphaned, which in turn leads to GARM terminating compute instances that are actually still in use.

This is just one scenario where having an agent reporting the state of the runner, both in terms of health, but also in terms of job activity, can help us keep the state in GARM more accurate, and reduce the number of API requests we need to make to the Forge and the IaaS.

## Features

The agent currently provides additional functionalities such as:

- **Remote Shell**: Allows users to open remote shell multiplexed over a websocket connection (optional, opt-in).
- **Heartbeats**: sends periodic heartbeats to the GARM server to indicate that the runner is alive and functioning properly.
- **Status Updates**: Sends status updates to the GARM server about the runner's state, such as when it starts or finishes a job, by watching the runner standard output/error.
- **Service Management**: Provides service management capabilities for the runner process. This is needed for the Gitea runner on Windows, but also facilitates the `Status Updates` feature for both GitHub and Gitea runners.

