
# SiteCheck

This project is a simple website uptime monitor. Designed to run on limited shared hosting where you can't have long
running servers, but you are allowed scheduled jobs. This monitor does a single set of checks, generates a static site
with the results, and exits, you then run it on a schedule with cron or similar to keep your monitoring up to date.

The actual checks are Lua scripts. There are a set of functions you can call to do various common checks. You then
go over the results and annotate them with a pass/degraded/failed status based on whatever criteria you wish.

By design you generally don't want your monitoring to be on the server(s) being monitored, but sometimes you need to be
able to check the status of internal services, docker containers with no public API, etc. To make that possible, this
application uses outposts, small stand alone applications that are designed to run resource checks. The core application
does all the work of tracking resource history, orchestrating the outposts, generating the static site, etc. The
outposts *only* run checks and reports their results.

The core application will run a local outpost that should be used for most checks, but remote ones are there to be used
as needed. More information about how to configure outposts can be found elsewhere in this readme.


## Building

Clone this repo and `go build ./cmd/sitecheck` and `go build ./cmd/scoutpost` . From there you can simply run
`sitecheck` and everything should "just work".

This is great for playing with the example resource check scripts, etc, but if you actually want to run this in
production you will likely want to copy the binaries, templates, and static files to somewhere else. From there you can
set up a web server to serve the output files, whatever cron solution you want to use to schedule runs, and any other
config you may need.


## Configuration

Configuration is all done with environment variables. If there is a `.env` file in the current working directory,
it will be loaded and variables will be read from it, however actual environment variables will always be preferred.

All variables have the default values shown below.


### Core (`sitecheck`)

These variables control the site generation and the database mostly. There is a worker pool for concurrent access to
outposts that you can play with if you want.

| Variable                    | Default             | Description                                                     |
|-----------------------------|---------------------|-----------------------------------------------------------------|
| `SITECHECK_OUTPOST_BIN`     | `./scoutpost`       | Path to the scoutpost binary for local checks                   |
| `SITECHECK_OUTPOST_WORKERS` | `4`                 | Max concurrent outpost connections                              |
| `SITECHECK_DEFAULT_TIMEOUT` | `30`                | Connection and check timeout (seconds)                          |
| `SITECHECK_RESOURCES_DIR`   | `resources`         | Check scripts directory (passed to local outpost)               |
| `SITECHECK_DB_PATH`         | `data/sitecheck.db` | SQLite database path                                            |
| `SITECHECK_TEMPLATES_DIR`   | `templates`         | HTML templates directory                                        |
| `SITECHECK_OUTPUT_DIR`      | `output`            | Static site output directory                                    |
| `SITECHECK_STATIC_DIR`      | `static`            | Static assets (CSS) directory                                   |
| `SITECHECK_SITE_TITLE`      | `SiteCheck Status`  | Site title                                                      |
| `SITECHECK_RETENTION_DAYS`  | `90`                | Days of history to keep                                         |
| `SITECHECK_GRAPH_WINDOWS`   | `24,168,720`        | Sparkline windows in hours                                      |
| `SITECHECK_NTFY_SERVER`     | *(empty)*           | ntfy server URL including topic, e.g. `https://ntfy.sh/mytopic` |


### Outpost (`scoutpost`)

Outposts run in one of two modes: CGI (1.1) mode, or server mode.

In CGI mode the scoutpost binary is a plain CGI program, and needs to be fronted with some sort of server, proxy, etc
that can run CGI binaries. Make sure you set the resource directory, and use a token if exposed to the internet!

In server mode, scoutpost creates a long running server process that listens for get requests on any path routed to it
(assuming the bearer token is correct). There is no TLS or anything, so if this is going to be routed over the internet,
make sure you put it behind a proxy and use a good token! The output is streaming JSONL, if that matters for any reason.
In server mode, you probably want to run it as a system service or something.

The outpost determines what mode it should be in by looking for the `GATEWAY_INTERFACE` environment variable. This
variable is part of the CGI spec, so if your server is wildly out of spec and doesn't send this you will have to work
around that (probably by fixing your server). 


| Variable                    | Default     | Description                        |
|-----------------------------|-------------|------------------------------------|
| `SITECHECK_TOKEN`           | *(empty)*   | Bearer token required from callers |
| `SITECHECK_RESOURCES_DIR`   | `resources` | Check scripts directory            |
| `SITECHECK_WORKERS`         | `4`         | Concurrent Lua check workers       |
| `SITECHECK_LISTEN`          | `:8080`     | Listen address (server mode only)  |
| `SITECHECK_DEFAULT_TIMEOUT` | `30`        | Check timeout (seconds)            |


## Linking Outposts

To tell the core application where its outposts are, you can create outpost scripts. These are .lua files that contain
a `meta()` function that must return a table that has information about the outpost.


```lua
{
    name        = "Example",
    url         = "https://example.com/cgi-bin/scoutpost",
    token       = "changeme",
    skip        = false,
    notify_down = true,
}
```

`name` is a user friendly name, used when reporting an outpost outage, etc. `url`, is the outpost URL. Simple. `token`
is the bearer token needed to talk to the outpost. The same token you give to the outpost via `SITECHECK_TOKEN`. If
`skip` is true, the outpost is skipped. Use for disabling and outpost if you need to, but don't want to remove it for
some reason. `notify_down`, if true, tells the core application to send a notification if it can't talk to the outpost.
This defaults to true, and you probably want to leave it that way.

Speaking of if an outpost is down... If an outpost id down, the core application will look through its historical data
to find all the resources that have reports from the downed outpost, if it finds any it will insert a dummy report
with the special "unknown" status. This report will show up on the generated site, but shouldn't be counted as a state
transition for notification purposes.


## Writing Check Scripts

Each `.lua` script in the resources directory is run as a check script. These scripts need two functions:
`meta()` and `check()`.

`meta()` is optional, and must return a table with meta-data about the resource check script. See below.

`check()` is required, and must return a user data type as returned from one of the provided test functions.


### `meta()` Values

All fields in the meta-data table are optional. The name defaults to the base name of the resource check script file,
skip defaults to false, and everything else defaults to an empty string.


```lua
{
    name        = "Example",
    description = "An example description",
    skip        = false,
    notify      = {
        pass     = true
        degraded = true,
        fail     = true,
    },
}
```

`skip`, if true, causes the script to be skipped as if it didn't exist. This is just here so I can ship a ton of example
scripts without making you delete them all, and so that you can disable a script quickly and nondestructively if you like.

The values in the `notify` table are boolean flags. If these are true, when the state of the resource transitions
from any state to the state the topic is for, there will be a message sent to the ntfy server if it is configured.
Repeated occurrences of the same state do not trigger notifications.


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
| `systemd_check(service, opts)`      | systemd service status check       |
| `exec_command(command, args, opts)` | Arbitrary command execution        |

Each of these functions returns a native value with a meta table that allows Lua to read some of the fields. One of
these return values **MUST** be returned from the `check()` function. You can return any of them, and you can even
call several of these in one check (I would suggest you don't) and just pick one to return.

Each of these return values have two keys you are intended to set:

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


**`exec_command(command, args, opts)`**

Run an arbitrary command and capture its exit code and output. The command runs directly (no shell)
unless you invoke `sh` or similar. Output is captured in three forms: stdout-only, stderr-only, and a
combined interleaved stream that preserves the original write ordering.

| Option    | Type   | Default        | Notes                              |
|-----------|--------|----------------|------------------------------------|
| `timeout` | number | config default | kill the command after N seconds   |
| `env`     | table  | *(current)*    | `{ ["KEY"] = "value", ... }`       |
| `stdin`   | string | `""`           | piped to command stdin             |

Returns:

| Field            | Type   | Notes                                          |
|------------------|--------|------------------------------------------------|
| `Command`        | string | full command line (for display)                |
| `ExitCode`       | int    | process exit code (`-1` if no exit occurred)   |
| `Stdout`         | string | standard output (truncated to 64 KiB in DB)    |
| `Stderr`         | string | standard error  (truncated to 64 KiB in DB)    |
| `Combined`       | string | interleaved stdout+stderr (truncated to 64 KiB)|
| `ResponseTimeMS` | float  | wall-clock milliseconds                        |
| `Error`          | string | execution error (timeout, not found, etc.)     |


### Other Custom Lua APIs

The resource check scripts load the Lua standard library as implemented by [the VM I am using](https://github.com/milochristiansen/lua),
which means there are a few minor things missing. Nothing that should matter, outside of a few features of the string
package. There is a full list of differences from standard Lua 5.3 in the readme for the VM.

Outside of that, there are also a few fully custom APIs provided to help do checks, these are listed here:


**`json.parse(string)`** and **`json.encode(value)`**

Parse or encode JSON. This should work pretty much exactly how you would expect. Generally these go to/from table values.
Look it's a JSON parser, it's not that complicated.


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
