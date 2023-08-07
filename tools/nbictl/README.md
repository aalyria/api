# nbictl -- NBI command-line tool

`nbctl` allows you to interact with the Spacetime NBI APIs from the command-line.

## Usage:

```sh
$ nbictl <action> [--operation-specific-flags]
```

### Configuration actions

#### generate-keys
```
Usage of nbictl generate-key:
  -country string
        country of certificate
  -dir string
        directory where you want your RSA keys to be stored. Default: ~/.nbictl/
  -location string
        location of certificate
  -org string
        [REQUIRED] organization of certificate
  -state string
        state of certificate
```

> **Note**
> After creating the Private-Public keypair, you will need to request API access by
> sharing the `.crt` file (a self-signed x509 certificate containing the public key) with
> Aalyria to receive the `USER_ID` and a `KEY_ID` needed to complete the `nbictl` configuration.

> **Warning** 
> Only share the public certificate (`.crt`) with Aalyria or third-parties.
> The private key (`.key`) must be protected and should never be sent by email
> or communicated to others in any.

#### set-context

You can create multiple contexts by specifying a name of context using `--context` flag.
If context name is not specified, the context will have name `DEFAULT`.

```
Usage of nbictl set-context:
  -context string
        context of NBI API environment (default "DEFAULT")
  -key_id string
        key id associated with the provate key provided by Aalyria
  -priv_key string
        path to your private key for authentication to NBI API
  -transport_security string
        transport security to use when connecting to NBI. Values: insecure, system_cert_pool
  -url string
        url of NBI endpoint
  -user_id string
        user id address associated with the private key provided by Aalyria
```

### NBI actions

#### create
```
Usage of nbictl create:
  -context string
        name of context you want to use
  -files path
        [REQUIRED] a path to the textproto file containing information of the entity you want to create
```
#### update
```
Usage of nbictl update:
  -context string
        name of context you want to use
  -files path
        [REQUIRED] a path to the textproto file containing information of the entity you want to update
```
#### delete
```
Usage of nbictl delete:
  -commit_time int
        [REQUIRED] commit timestamp of the entity you want to delete (default -1)
  -context string
        name of context you want to use
  -id string
        [REQUIRED] the id of the entity you want to delete
  -type string
        [REQUIRED] type of entities you want to delete. list of possible types: [STATION_SET NETWORK_NODE CDPI_STREAM_INFO DRAIN_PROVISION INTENT INTERFACE_LINK_REPORT MOTION_DEFINITION NETWORK_STATS_REPORT BAND_PROFILE COMPUTED_MOTION TRANSCEIVER_LINK_REPORT ANTENNA_PATTERN PLATFORM_DEFINITION INTERFERENCE_CONSTRAINT PROPAGATION_WEATHER SERVICE_REQUEST DEVICES_IN_REGION SURFACE_REGION]
```
#### list
```
Usage of nbictl list:
  -context string
        name of context you want to use
  -type string
        [REQUIRED] type of entities you want to query. list of possible types: [DEVICES_IN_REGION NETWORK_STATS_REPORT PLATFORM_DEFINITION PROPAGATION_WEATHER SERVICE_REQUEST COMPUTED_MOTION ANTENNA_PATTERN BAND_PROFILE INTENT NETWORK_NODE TRANSCEIVER_LINK_REPORT CDPI_STREAM_INFO STATION_SET SURFACE_REGION DRAIN_PROVISION INTERFACE_LINK_REPORT INTERFERENCE_CONSTRAINT MOTION_DEFINITION]
```
