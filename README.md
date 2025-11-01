# Go WOL Proxy (FORK)


# Background

I run JellyFin inside a Proxmox LXC while keeping the multimedia content in a OpenMediaVault NAS.
This NAS is an old PC loaded with lots of disks which makes this setup not energy efficient.
This fork aims to address my specific case, sending WoL magic packet to one or multiple hosts while keeping a reverse proxy for the main destination.


This proxy runs in between my main reverse proxy and the Jellyfin host. To prevent the main proxy from triggering the magic packet I added / Sessions to the ignoref paths list.

A Wake-on-LAN proxy service written in Go that automatically wakes up servers when requests are made to them.

## Features

- Forwards HTTP requests to a single configured destination (if online)
- Checks multiple backends for health
- Sends optional WOL packets to offline backends (configurable)
- Caches health status to minimize latency for frequent requests
- Configurable via TOML config file
- Can ignore certain hosts and/or paths

## Removed Features

[X] No docker image

[X] No shutdown (omv does this with a plugin)

## Configuration

The service is configured using a TOML file. Here's an example configuration:

```toml
[proxy]
listenPort = ":8096"                          # Port to listen on
mainHostKeyword = "video.example.com"         # Host header keyword for requests
destination = "http://192.168.1.240:8096"     # URL to forward traffic to
skipCheckTimeout = 30                         # seconds; skip health checks if backends are recently online

[backends.mainService]
destination = "http://192.168.1.240:8096"
macAddress = "AA:AA:BB:BB:CC:CC"
broadcastIP = "192.168.1.255"
wolPort = 9
ignoredHosts = []
ignoredPaths = ["/Sessions"]
wolEnable = false

[backends.dataStore]
destination = "http://192.168.1.250"
macAddress = "DD:EE:FF:00:11:22"
broadcastIP = "192.168.1.255"
wolPort = 9
ignoredHosts = []
ignoredPaths = ["/Sessions"]
wolEnable = true
```
