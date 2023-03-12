# Spacetime's APIs

Spacetime has 2 APIs:
- The **Northbound Interface (NBI)** allows humans or applications to define and orchestrate a network. This includes functions such as specifying the time-dynamic position and orientation of platforms and antennas, defining networking parameters on each node in the network, and creating requests for service that will be scheduled and routed through the network. 
- The **Control to Data-plane Interface (CDPI)**, or Southbound Interface, allows Spacetime to control network devices and receive updates in return. This includes functions such as steering antenna beams to establish new links and configuring RF parameters like the transmit power and channel. 

## Developer Guides
You can find developer guides and tutorials on [this website](docs.spacetime.aalyria.com).

This site contains:
- [NBI Developer Guide](https://docs.spacetime.aalyria.com/nbi-developer-guide) 
- [CDPI Developer Guide](https://docs.spacetime.aalyria.com/cdpi-developer-guide)
- [Building a Scenario Tutorial](https://docs.spacetime.aalyria.com/scenario-building)
- [Authentication](https://docs.spacetime.aalyria.com/authentication)

## Repo Contents
In this repo, you will find the following directories:
- [api](/api): The [gRPC](https://grpc.io/) and [Protocol Buffers](https://protobuf.dev/) definitions of the API.
- [cdpi_agent](/cdpi_agent): A Go implementation of a CDPI agent.
- [contrib](/contrib): An open-source directory of real hardware that has been modeled in Spacetime and used in real networks. Contributions are welcome!  

## Contributing
Spacetime welcomes contributions to its APIs. Read the [governance](GOVERNANCE.md) document to learn more about policies and processes for suggesting changes to the APIs.

### License
Spacetime's APIs are licensed under Apache 2.0 (see the [LICENSE](./LICENSE) file).
