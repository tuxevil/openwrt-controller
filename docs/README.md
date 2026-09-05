# OMEGA Documentation

This directory contains the detailed guides behind the project README. Start with the document that matches the task:

| Document | Use it when you need to... |
|---|---|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Understand components, boundaries and data flow |
| [DEPLOYMENT.md](DEPLOYMENT.md) | Install locally or run the native systemd service |
| [OPERATIONS.md](OPERATIONS.md) | Perform previews, rollouts, backups and recovery |
| [SECURITY_MODEL.md](SECURITY_MODEL.md) | Review authentication, tenant isolation and secrets |
| [DEVICE_AGENT.md](DEVICE_AGENT.md) | Install, enroll or troubleshoot an OpenWrt device |
| [TESTING.md](TESTING.md) | Run quality gates or opt-in integration checks |

Related references:

- [OpenAPI contract](../openapi.yaml)
- [Operator security checklist](../SECURITY.md)
- [Contribution workflow](../CONTRIBUTING.md)
- [Changelog](../CHANGELOG.md)
- [OpenWrt capability notes](OpenWrt_Capabilities.md) (reference material when present)

Documentation describes the current repository behavior. When a guide and implementation disagree, treat the code and tests as the source of truth and open an issue to correct the documentation.
