# Telegraf input plugin for ce102m electricity meters.
[![Coverage Status](https://coveralls.io/repos/github/srgsf/ce102m-telegraf-plugin/badge.svg)](https://coveralls.io/github/srgsf/ce102m-telegraf-plugin)
[![lint and test](https://github.com/srgsf/ce102m-telegraf-plugin/actions/workflows/golint-ci.yaml/badge.svg)](https://github.com/srgsf/ce102m-telegraf-plugin/actions/workflows/golint-ci.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/srgsf/ce102m-telegraf-plugin)](https://goreportcard.com/report/github.com/srgsf/ce102m-telegraf-plugin)

This is a [ce102m](http://www.energomera.ru/en/products/meters/ce102mr5) electricity meter input plugin for Telegraf, meant to be compiled separately and used externally with telegraf's execd input plugin.

It reads channel current values and health status from the meter using [62056-21](https://github.com/srgsf/iec62056.golang) protocol wrapper via tcp.

## Install Instructions
Download [release](https://github.com/srgsf/ce102m-telegraf-plugin/releases) for your target architectrue.

Extract archieve and edit plugin.conf file.

You should be able to call this from telegraf now using execd:

```toml
[[inputs.execd]]
  command = ["/path/to/ce102m", "-config", "plugin.conf", "-poll_interval", "1m"]
  signal = "none"

# sample output: write metrics to stdout
[[outputs.file]]
  files = ["stdout"]
```

## Build from sources

Download the repo somewhere

    $ git clone https://github.com/srgsf/ce102m-telegraf-plugin.git

Build the binary for your platform using make

    $ make build

The binary will be avalilable at ./dist/ce102m


## Plugin configuration example

```toml
## Gather data from ce102m power meter ##
[[inputs.ce102m]]
    ## tcp socket address for rs485 to ethernet converter.
    socket ="localhost:4001"
    ## device address - optional for broadcast.
    # address = ""
    ## If even parity should be handled manually.
    software_parity = true
    ## Status request interval - don't request if ommited or 0
    status_interval = "1d"
    ## Timezone of device system time.
    systime_tz = "Europe/Moscow"
    ## should protocol be logged as debug output.
    # log_protocol = true
    ## log level. Possible values are error,warning,info,debug
    #log_level = "info"
    ## query only the following tariffs starts with 1 for summary.
    tariff_include = [2,3]
    ## value prefix for a tariff
    tariff_prefix = "chan_"
```
