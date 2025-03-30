# tailscale

## Name

*tailscale* - serves DNS records for Tailscale nodes with subdomain resolution support.

## Description

The tailscale plugin serves DNS records for Tailscale nodes in your Tailnet. It automatically creates A and AAAA records for all Tailscale machines and supports CNAME records via Tailscale tags. Additionally, it provides subdomain resolution - any subdomain of a registered Tailscale machine will resolve to the same IP address as the base machine.

This plugin allows:
- Integrating Tailscale machines into your existing DNS domain
- Creating CNAME records via Tailscale node tags
- Resolving arbitrary subdomains of Tailscale machines (wildcard-like behavior)

The plugin retrieves node information through the local machine's Tailscale socket, so only machines visible to the hosting Tailscale node (visible in `tailscale status`) will be included in DNS responses.

## Syntax

```
tailscale ZONE
```

* **ZONE** is the zone that plugin should be authoritative for.

## Metrics

If monitoring is enabled (via the *prometheus* directive) the following metrics are exported:

* `coredns_tailscale_requests_total{server,type}` - count of DNS requests processed by record type
* `coredns_tailscale_responses_total{server,rcode}` - count of DNS responses by return code
* `coredns_tailscale_request_duration_seconds{server}` - histogram of request processing time
* `coredns_tailscale_nodes_total{server}` - number of Tailscale nodes in the Tailnet

The `server` label indicates which server handled the request, the `type` label indicates the DNS record type requested (A, AAAA, CNAME, etc.), and the `rcode` label indicates the DNS response code (NOERROR, NXDOMAIN, etc.).

## Ready

This plugin reports readiness to the ready plugin once it has successfully loaded the Tailscale node information.

## Examples

Enable tailscale plugin for the `example.com` zone:

```
example.com {
  tailscale example.com
  log
  errors
}
```

With this configuration:
1. Tailscale machines will resolve as `machinename.example.com`
2. Machines with Tailscale tags like `cname-app` will create `app.example.com` records pointing to that machine
3. Subdomains like `web.machinename.example.com` will resolve to the same IP as `machinename.example.com`

Enable metrics for monitoring:

```
example.com {
  tailscale example.com
  prometheus
  log
  errors
}
```

This configuration:
1. Serves DNS records for Tailscale machines on `example.com`
2. Exports Prometheus metrics for the tailscale plugin
3. Makes metrics available at http://localhost:9153/metrics

## CNAME Records via Tailscale Tags

A CNAME record can be created by adding a Tailscale machine tag prefixed with `cname-`. The text after the prefix becomes the hostname:

* Machine `server1` with tag `cname-app` creates:
  ```
  app.example.com IN CNAME server1.example.com.
  server1.example.com IN A <Tailscale IPv4>
  server1.example.com IN AAAA <Tailscale IPv6>
  ```

## Subdomain Resolution

Any subdomain of a Tailscale machine or CNAME will resolve to the same IP address:

```
server1.example.com          → <Tailscale IP>
web.server1.example.com      → <Same Tailscale IP>
api.web.server1.example.com  → <Same Tailscale IP>

app.example.com              → <Tailscale IP via cname-app tag>
admin.app.example.com        → <Same Tailscale IP via cname-app tag>
```

This is particularly useful for:
- Running multiple services on the same Tailscale machine
- Creating wildcard-like behavior without actual wildcard DNS records
- Simplifying service discovery within a Tailnet

## Also See

See the [CoreDNS manual](https://coredns.io/manual) and the [original repository](https://github.com/ShrewdHydra/coredns-tailscale) this fork is based on.
