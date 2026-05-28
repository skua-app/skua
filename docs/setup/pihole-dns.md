# Optional: Pi-hole — Local DNS Record

This is one example of pointing a friendly hostname at your Skua
reverse proxy without going through public DNS. The default LAN-only
deployment is fine with the host's IP address in the browser. Any
local-DNS solution works — Pi-hole, Unbound, your router's DNS override,
or even `/etc/hosts` for a single client. The steps below are
Pi-hole-specific.

Add a local A-record so that `skua.example.com` resolves to the
reverse-proxy host from anywhere on the network.

## Steps (Pi-hole Admin UI)

1. Open Pi-hole admin: `http://<pihole-primary>/admin` (primary) or
   `http://<pihole-backup>/admin` (backup).
2. Go to **Local DNS → DNS Records**.
3. Add record:
   - Domain: `skua.example.com`
   - IP: `<reverse-proxy>`
4. Click **Add**.

Repeat on **both** Pi-hole instances. If nebula-sync is configured, adding it
on the primary is enough, but adding it to both is safer in case sync is
delayed or paused.

## Verify

```bash
# From a client on the network:
dig skua.example.com @<pihole-primary>
# Expected: <reverse-proxy>
```
