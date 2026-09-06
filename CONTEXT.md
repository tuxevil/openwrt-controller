# Network Identity Context

## Glossary

- **Node**: a controller-managed Gateway or access point identified by its device ID/MAC.
- **Client**: an endpoint observed by a node through Wi-Fi stations, ARP, bridge, or DHCP telemetry.
- **Network identity**: the human-readable label and observed attributes associated with a node or client MAC.
- **Trusted client**: a site-scoped, operator-declared client identity that represents an expected administrative origin. It is contextual evidence, not authentication.

## Safety invariants

- MAC addresses remain available as secondary identifiers, but reports lead with a label whenever one is known.
- Trust is scoped to a site and may expire; it reduces suspicion for expected administrative activity but never suppresses failed authentication, credential abuse, persistence, or other independent indicators.
- A MAC can be spoofed or randomized. Trust must not authorize a device or permit an automatic configuration change.
