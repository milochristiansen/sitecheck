
# SiteCheck

This project is a simple website uptime monitor. Designed to run on limited shared hosting where you can't have long
running servers, but you are allowed scheduled jobs. This monitor does a single set of checks, generates a static site
with the results, and exits, you then run it on a schedule with cron or similar to keep your monitoring up to date.

The actual checks are Lua scripts. There are a set of functions you can call to do various common checks. You then
go over the results and annotate them with a pass/degraded/failed status based on whatever criteria you wish.


## Configuration

Configuration is all done with environment variables. If there is a `.env` file in the current working directory,
it will be loaded and variables will be read from it, however actual environment variables will always be preferred.

All variables have the default values shown below.

```env
SITECHECK_WORKERS=4
SITECHECK_DEFAULT_TIMEOUT=30
SITECHECK_DB_PATH=data/sitecheck.db
SITECHECK_RESOURCES_DIR=resources
SITECHECK_TEMPLATES_DIR=templates
SITECHECK_OUTPUT_DIR=output
SITECHECK_STATIC_DIR=static
SITECHECK_SITE_TITLE=SiteCheck Status
SITECHECK_RETENTION_DAYS=90
SITECHECK_GRAPH_WINDOWS=24,168,720
SITECHECK_NTFY_SERVER=https://ntfy.sh
```

## Writing Check Scripts

Each `.lua` script in the resources directory is run as a check script. These scripts need two functions:
`meta()` and `check()`.

`meta()` is optional, and must return a table with meta-data about the resource check script. See below.

`check()` is required, and must return a user data type as returned from one of the provided test functions.


### `meta()` Values

All fields in the meta-data table are optional. The name defaults to the base name of the resource check script file,
and everything else defaults to an empty string.


```lua
{
    name        = "Example",
    description = "An example description",
    notify      = {
        pass     = "<your ntfy topic>",
        degraded = "<your ntfy topic>",
        fail     = "<your ntfy topic>",
    },
}
```

The values in the `notify` table are topics for ntfy. If these are provided, when the state of the resource transitions
from any state to the state the topic is for, there will be a message sent to the provided topic. Repeated occurrences
of the same state do not trigger notifications.


### Pass Constants

The following constants are provided to use when setting the `Pass` value in the `check()` function return value.

| Constant   | Meaning                       |
|------------|-------------------------------|
| `FAIL`     | Check failed                  |
| `DEGRADED` | Check succeeded with warnings |
| `PASS`     | Check passed                  |


### Test Functions

| Function                            | Description                        |
|-------------------------------------|------------------------------------|
| `http_fetch(url, opts)`             | HTTP/HTTPS request                 |
| `icmp_ping(host, opts)`             | ICMP ping (privileged) or UDP ping |
| `tcp_connect(host, port, opts)`     | TCP connectivity check             |
| `dns_lookup(host, opts)`            | DNS resolution                     |
| `ssl_certificate(host, port, opts)` | TLS certificate inspection         |
| `systemd_check(service, opts)`         | systemd service status check       |

Each of these functions returns a native value with a meta table that allows Lua to read some of the fields. One of
these return values **MUST** be returned from the `check()` function. You can return any of them, and you can even
call several of these in one check (I would suggest you don't) and just pick one to return.

Each of these return values have two keys you are intended to check:

* `Pass` defaults to `FAIL`, and is to be set by you to indicate the result of the check (see above for constants that
  should be used to set the value).
* `FailReason` defaults to an empty string and should be set with a descriptive reason the check failed if the result of
  `Pass` is not `PASS`.

Other return values (and the function options) are documented below.


**`http_fetch(url, opts)`**

Fetch a URL and return the response.

| Option                 | Type   | Default        | Notes                         |
|------------------------|--------|----------------|-------------------------------|
| `method`               | string | `"GET"`        | HTTP method                   |
| `headers`              | table  | `{}`           | `{ ["name"] = "value", ... }` |
| `body`                 | string | `""`           | request body                  |
| `timeout`              | number | config default | seconds                       |
| `follow_redirects`     | bool   | `true`         |                               |
| `max_redirects`        | number | `10`           |                               |
| `insecure_skip_verify` | bool   | `false`        | skip TLS verification         |

Returns:

| Field            | Type   | Notes                         |
|------------------|--------|-------------------------------|
| `URL`            | string | requested URL                 |
| `StatusCode`     | int    | HTTP status code              |
| `Body`           | string | response body                 |
| `BodySize`       | int    | response body bytes           |
| `ResponseTimeMS` | float  | milliseconds                  |
| `TLSVersion`     | string | TLS version if HTTPS          |
| `RemoteIP`       | string | remote IP address             |
| `RedirectCount`  | int    | redirects followed            |
| `Error`          | string | error message                 |


**`icmp_ping(host, opts)`**

Ping the given host. Raw ICMP needs root or similar, but on Linux/Darwin it may be possible to use a UDP thingy that
lets you send ICMP packets via some magic BS that seems to work and I haven't really looked into the details of.

| Option       | Type   | Default        | Notes                                                 |
|--------------|--------|----------------|-------------------------------------------------------|
| `count`      | number | `3`            | packets to send                                       |
| `timeout`    | number | config default | seconds                                               |
| `privileged` | bool   | `false`        | true for real ICMP, false for UDP (Darwin/Linux only) |

Returns:

| Field             | Type   | Notes                         |
|-------------------|--------|-------------------------------|
| `Host`            | string | target host                   |
| `PacketsSent`     | int    | packets sent                  |
| `PacketsReceived` | int    | packets received              |
| `PacketLossPct`   | float  | loss percentage               |
| `MinMS`           | float  | min RTT (ms)                  |
| `MaxMS`           | float  | max RTT (ms)                  |
| `ResponseTimeMS`  | float  | average RTT (ms)              |
| `Error`           | string | error message                 |


**`tcp_connect(host, port, opts)`**

Make a TCP connection and then close it.

| Option    | Type   | Default        |
|-----------|--------|----------------|
| `timeout` | number | config default |

Returns:

| Field            | Type   | Notes                         |
|------------------|--------|-------------------------------|
| `Host`           | string | target host                   |
| `Port`           | int    | target port                   |
| `ResponseTimeMS` | float  | milliseconds                  |
| `RemoteIP`       | string | remote IP address             |
| `Error`          | string | error message                 |


**`dns_lookup(host, opts)`**

Lookup a host name via DNS.

| Option     | Type   | Default        | Notes         |
|------------|--------|----------------|---------------|
| `timeout`  | number | config default |               |
| `resolver` | string | system default | DNS server IP |

Returns:

| Field            | Type   | Notes                         |
|------------------|--------|-------------------------------|
| `Host`           | string | looked-up host                |
| `IPs`            | table  | array of IP strings           |
| `ResponseTimeMS` | float  | milliseconds                  |
| `Error`          | string | error message                 |


**`ssl_certificate(host, port, opts)`**

Get a site's SSL certificate.

| Option                 | Type   | Default        | Notes                 |
|------------------------|--------|----------------|-----------------------|
| `timeout`              | number | config default |                       |
| `insecure_skip_verify` | bool   | `false`        | skip TLS verification |

Returns:

| Field            | Type   | Notes                         |
|------------------|--------|-------------------------------|
| `Host`           | string | target host                   |
| `Port`           | int    | target port                   |
| `Issuer`         | string | certificate issuer            |
| `Subject`        | string | certificate subject           |
| `NotBefore`      | string | valid-from (RFC 3339)         |
| `NotAfter`       | string | valid-until (RFC 3339)        |
| `DaysRemaining`  | int    | days until expiry             |
| `ResponseTimeMS` | float  | milliseconds                  |
| `Error`          | string | error message                 |



**`systemd_check(service, opts)`**

Query a systemd service's status via D-Bus. Requires a systemd-based Linux host and D-Bus access
(read-only unit status is available to unprivileged users on most distributions).

| Option    | Type   | Default        | Notes                         |
|-----------|--------|----------------|-------------------------------|
| `timeout` | number | config default | D-Bus call timeout in seconds |

Returns:

| Field            | Type   | Notes                                                        |
|------------------|--------|--------------------------------------------------------------|
| `ServiceName`    | string | unit name (e.g. `nginx.service`)                             |
| `ActiveState`    | string | `active`, `inactive`, `failed`, `activating`, `deactivating` |
| `SubState`       | string | `running`, `dead`, `exited`, `auto-restart`, etc.            |
| `LoadState`      | string | `loaded`, `not-found`, `masked`, etc.                        |
| `MainPID`        | int    | main process PID (0 if none)                                 |
| `ResponseTimeMS` | float  | milliseconds                                                 |
| `Error`          | string | error message (D-Bus or unit lookup failure)                 |


## Generated Site

```
output/
├── index.html           # overview: status summary, resource cards, sparklines
├── resources/
│   └── <slug>.html      # detail: stats, SVG line chart, recent checks table
└── static/
    └── style.css
```

Serve `output/` with any static file server. Everything should work if you just open the files directly as well.


## Database

Check results are stored in SQLite (`data/sitecheck.db` by default). Old rows are purged automatically based on
`SITECHECK_RETENTION_DAYS` (default 90). You can safely ignore this pretty much always. It only exists because I have to
store historical data somewhere to make the graphs and stuff work.
