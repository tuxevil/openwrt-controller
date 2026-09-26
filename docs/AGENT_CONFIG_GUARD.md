# Confirmed agent configuration changes

`devices/agent-config-guard.sh` changes only `/etc/nerve/agent.conf`. It does
not protect UCI, routes, firewall rules, credentials, certificates, firmware,
or other files. Those changes require their own recovery plan.

Install the script as `/usr/sbin/agent-config-guard` and the accompanying
`agent-config-guard.init` as `/etc/init.d/nerve-config-guard`, both root-owned
and executable. Enable and start the service before using `arm` or `apply`.
Verify `ubus call service list '{"name":"nerve-config-guard"}'` reports a
running instance. Do not replace the guard during a pending change.

The procd worker survives SSH disconnection and is respawned after process
death. A durable journal records the original file, its digest, the proposed
file, boot identity and an uptime-based deadline. S18 startup restores an
unconfirmed change from an earlier boot before the agent's S99 startup.
No controller connection is required for recovery. A failed restore keeps
the journal active and retries; successful confirmation and restoration
retain separate root-only evidence directories.

After staging a candidate configuration and validating its destination:

```sh
/usr/sbin/agent-config-guard arm unique-change-id 600 /tmp/agent.conf.candidate
/usr/sbin/agent-config-guard apply unique-change-id
```

Use a new SSH session to verify the agent process, authenticated telemetry,
configuration retrieval, certificate validation, Internet/DNS, management
access and unchanged UCI. Then, before the deadline:

```sh
/usr/sbin/agent-config-guard confirm unique-change-id
```

Omitting confirmation restores the previous file and restarts only `agent`.
An external health monitor can request immediate recovery with
`agent-config-guard rollback unique-change-id`. It verifies the active identity
and expires its deadline before restoring, so interruption cannot leave that
change confirmable and the supervised worker can retry recovery.
Use a new ID after a terminal result. Never change multiple fleet members
at once; complete canaries before touching a gateway. Confirm only after
all required health checks succeed. Stage the certificate separately and
keep the previous controller endpoint available throughout migration.

Run the fixture checks with `sh devices/agent-config-guard-test.sh`. They
exercise expiry, reboot identity changes, confirmation fencing, restoration
retry and candidate integrity without changing the host configuration.
Fixture reboot checks do not establish recovery from physical flash failure
or from a router that cannot boot. A live canary must separately establish
supervision and automatic expiry recovery on the deployed firmware.

The `GUARD_*` environment variables exist for isolated fixtures; production
service execution uses the fixed defaults. The guard assumes local root and
the candidate file are trusted. It provides rollback, not configuration
validation or protection against a privileged attacker.
