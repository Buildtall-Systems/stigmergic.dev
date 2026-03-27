---
title: meta.nostr.io Admin UI Mockup
---

# meta.nostr.io — Admin UI Mockup

Wireframe mockups for the metadata manager admin interface (buildtall#42).

## Login Screen

```wiremd
::: hero
## meta.nostr.io
Metadata aggregation for Nostr
Sign in with your Nostr identity to manage the metadata mirror.

[Login with NIP-07 Extension]{.primary}
:::
```

## Dashboard

```wiremd
::: navbar
## meta.nostr.io
[Mirror Status]  [Watchlist]  [Event Log]  [Logout]{.secondary}
:::

::: grid {columns=4}
::: card
### Watched Users
47
:::
::: card
### Connected Relays
12
:::
::: card
### Events Mirrored
2,841
:::
::: card
### Last Event
3 min ago
:::
:::

::: form
### Add User to Watchlist
[____________________________]
[Add User]{.primary}
:::

::: card
### npub1mkq...0r4tx
rob@buildtall.systems
kind 0, 10002 — last seen 2 min ago
[Remove]{.danger}
:::

::: card
### npub1sg6p...ekt5e
alice@nostr.com
kind 0, 10002 — last seen 14 min ago
[Remove]{.danger}
:::

::: card
### npub1r45p...l04zht
bob@example.com
kind 0, 10002 — last seen 1 hour ago
[Remove]{.danger}
:::

::: card
### npub139dk...qy9s
carol@relay.damus.io
kind 0, 10002 — last seen 3 hours ago
[Remove]{.danger}
:::

[Previous]  [Next]
```

## Relay Status Panel

```wiremd
::: navbar
## Relay Connections
:::

::: grid {columns=2}
::: card
### wss://relay.damus.io
connected — 847 events mirrored
:::
::: card
### wss://nos.lol
connected — 612 events mirrored
:::
::: card
### wss://purplepag.es
connected — 531 events mirrored
:::
::: card
### wss://relay.nostr.band
connected — 419 events mirrored
:::
::: card
### wss://delos.nostr.io
connected — 298 events mirrored
:::
::: card
### wss://relay-new.drss.io
connected — 134 events mirrored
:::
:::
```

## Remove User Confirmation

```wiremd
::: modal
## Remove User
Remove npub1mkq...0r4tx from the watchlist?
This will stop mirroring their kind 0 and 10002 events.
[Cancel]  [Confirm Removal]{.danger}
:::
```
