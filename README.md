# Spacetime's APIs

Spacetime offers 3 APIs:

1. The **Northbound Interface (NBI)** allows humans or applications to define and orchestrate a network. This includes functions such as specifying the time-dynamic position and orientation of platforms and antennas, defining networking parameters on each node in the network, and creating requests for service that will be scheduled and routed through the network. [Learn more](api/nbi/)
2. The **Southbound Interface (SBI)** is the collection of services through which devices participating in the network communicate with Spacetime. This includes services through which these devices may receive schedule updates from Spacetime, and through which they may push metrics and observations to Spacetime. [Learn more](api/scheduling/)
3. The **Federation API**, or East-West Interface, allows peer networks to request and to supply network resources and interconnections between partners’ networks. This facilitates dynamic, real-time inter-network connections, which allows operators to automatically and quickly supplement gaps in network coverage or advertise unused capacity to make full use of underutilized assets. [Learn more](api/federation/)

## Developer Guides
You can find developer guides and tutorials on [this website](https://docs.spacetime.aalyria.com).

This site contains:
- [NBI Developer Guide](https://docs.spacetime.aalyria.com/api/nbi) 
- [SBI Developer Guide](https://docs.spacetime.aalyria.com/api/sbi)
- [Building a Scenario Tutorial](https://docs.spacetime.aalyria.com/api/nbi/build-your-first-scenario/)
- [Authentication](https://docs.spacetime.aalyria.com/api/authentication/)

## Repo Contents
In this repo, you will find the following directories:
- [api](/api): The [gRPC](https://grpc.io/) and [Protocol Buffers](https://protobuf.dev/) definitions of the API.
- [agent](/agent): A Go implementation of an SBI agent.
- [contrib](/contrib): An open-source directory of real hardware that has been modeled in Spacetime and used in real networks. Contributions are welcome!  

## Contributing
Spacetime welcomes contributions to its APIs. Read the [governance](GOVERNANCE.md) document to learn more about policies and processes for suggesting changes to the APIs.

### License
Spacetime's APIs are licensed under Apache 2.0 (see the [LICENSE](./LICENSE) file).
