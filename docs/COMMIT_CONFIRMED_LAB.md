# Commit-Confirmed Lab

## Scope

Operator authorized a disposable test node: `10.128.128.3`, MAC
`04:a1:51:96:a6:4d`, Netgear WNDR3800CH. The operator has an external backup.
The gateway and other AP are excluded. The test node remains on a shared LAN;
disposable does not authorize conflicting DHCP, IP addresses or bridge loops.

Tracker: `openwrt-controller-z7nb`. Production executor: `openwrt-controller-01lq`.

## Hostname Experiment: 2026-09-05

Firmware: OpenWrt 25.12.5, revision `r33051-f5dae5ece4`, kernel 6.12.94.
Final read-only verification: approximately 13:32 -05:00.

Run from repository root, only with operator authorization:

```bash
bash devices/commit-confirmed-hostname-lab.prototype.sh --execute
```

This is a lab prototype, not a production execution path. It uses existing SSH
host-key verification, pins the destination and MAC, rejects pre-existing UCI
changes/rpcd staging, and grants its temporary rpcd session access only to
`system`. A temporary configuration-copy edit/revert precedes the live test.
The session ID is not logged or stored in this report.

Exact change: `system.@system[0].hostname`, `wndr3800ch` to `wndr3800ch-lab`.
Apply uses native `ubus uci apply`, `rollback=true`, `timeout=20`. There is no
explicit network restart and no controller restart/deployment.

| Experiment | Observed Result |
| --- | --- |
| Apply, end originating SSH session, omit confirmation | Configured and running hostname changed, then both returned to original after deadline |
| Apply, confirm from a new SSH session, wait beyond deadline | Configured and running hostname remained changed |
| Restore original through another native apply/confirm | Configured and running hostname returned to original |
| Compare six configuration files | `network`, `dhcp`, `firewall`, `dropbear`, `system`, `wireless` matched their original SHA-256 hashes byte-for-byte |
| Cleanup | No pending global UCI changes, lab rpcd staging or fixture directories found |
| LAN/controller check | Node LAN remained up at `10.128.128.3`; controller remained active |

The first invocation stopped in read-only preflight because UCI stores the LAN
address as `10.128.128.3/24`, not `10.128.128.3`. The explicit expected value was
corrected before the successful experiment. No live apply failed.

## Network Experiment: 2026-09-05

Operator explicitly approved loss of management access on the disposable node.
Exact change: `network.lan.proto`, `static` to `none`. The LAN address option,
DHCP, firewall, bridge membership, SSH configuration and other devices were not
edited. Native rpcd apply used `rollback=true`, `timeout=60`, and an isolated
session granted access only to `network`. No confirmation was sent.

Reproduction (each execution requires fresh operator approval):

```bash
bash devices/commit-confirmed-network-lab.prototype.sh --check
bash devices/commit-confirmed-network-lab.prototype.sh --execute
```

The check validates identity, initial LAN state, absence of pending changes and
rpcd transactions, then performs shell syntax validation and edit/revert on a
temporary configuration copy. The executor applies once, never retries apply,
and requires repeated failed SSH probes followed by restored runtime LAN state.
RPC errors alone do not count as proof of lost SSH connectivity.

Observed output from the single live apply:

```text
Native apply acknowledged.
OBSERVED t=10s: SSH management access unavailable.
OBSERVED t=105s: SSH reachable again, static LAN address/default route restored.
PASS: loss and automatic recovery observed; same boot/rpcd PID, six config hashes unchanged.
PASS cleanup: lab session removed; all six config files match baseline.
```

Times are probe observations relative to the local apply invocation, not precise
network transition timestamps. The test sent no manual rollback, reapply,
confirmation or reboot during recovery. Cleanup happened after automatic recovery
was verified and removed only the lab session and its restored pending delta.

Final verification around 13:44-13:45 -05:00:

- LAN up with static `10.128.128.3/24` and default route via `10.128.128.1`.
- All six configuration files byte-identical to the pre-test baseline.
- Same boot ID and rpcd PID (`1187`); no reboot or rpcd restart observed.
- No pending global UCI changes or remaining lab staging/fixture artifacts.
- Both radios up, neither pending nor disabled; all five Wi-Fi interfaces `psk2`.
- Controller active; all three devices still `SYNCED`, generations unchanged.
- Gateway and other AP each answered all 90 monitoring pings with zero loss.
  Those samples covered part of the experiment, not the entire recovery window
  and not end-user Internet/DNS functionality. The restored node answered 3/3
  follow-up pings.

Read-only netifd logs show LAN reconfiguration at 13:41:46-13:41:47 and again at
13:43:08-13:43:10, including bridge/link transitions and radio reconfiguration.
Radio setup emitted `Not supported (-122)` warnings; the subsequent wireless
status confirmed both radios up. The warnings were not separately diagnosed.

**Timing finding:** a requested 60-second native rollback window did not imply
management access within 60 seconds. This run observed restored access at 105
seconds. The exact source of the additional delay is not established. Production
must distinguish apply/confirmation deadlines from a bounded recovery-observation
window and verify runtime state instead of assuming restoration at timer expiry.

## What This Establishes

On this firmware, the hostname experiments establish that native confirmation
cancels the rollback window and that rollback does not require the originating
SSH session. The network experiment additionally demonstrates loss and automatic
recovery of management access for the exact `static` to `none` change. A device
can therefore restore this configuration without a controller connection during
the outage. This does not establish the same guarantee for every mutation.

## Remaining-Fault Experiments: 2026-09-05

Operator requested validation of the remaining guarantees and then explicitly
asked to continue. All operations were restricted to the disposable node. Each
experiment used only `system.@system[0].hostname`, `wndr3800ch` to
`wndr3800ch-lab`, with a 60-second native rollback window. Crash/reboot recovery
was observed before any explicit repair. No additional network-loss test or
physical power interruption was performed.

Reproduction, with fresh operator authorization and sufficient runner time
(allow at least 400 seconds for a fault mode, including recovery/cleanup):

```bash
bash devices/commit-confirmed-hostname-lab.prototype.sh --check
bash devices/commit-confirmed-hostname-lab.prototype.sh --execute-replay
bash devices/commit-confirmed-hostname-lab.prototype.sh --execute-rpcd-crash
bash devices/commit-confirmed-hostname-lab.prototype.sh --execute-reboot
```

Do not run the fault modes concurrently. They intentionally return failure when
the original hostname is not recovered automatically, even when subsequent
explicit restoration succeeds.

### Duplicate Requests and Confirmation

- A duplicate `apply` while rollback was pending returned `Permission denied`.
- Confirmation using a different lab session returned `Permission denied`.
- Confirmation by the initiating session succeeded.
- Repeating that confirmation returned `No response` (CLI exit 251), not the
  previous terminal result. The prototype does not log the raw error containing
  the session credential.
- The confirmed hostname and configuration hashes remained unchanged beyond the
  rollback window despite the duplicate/foreign requests.

This establishes rejection/isolation for these cases, not durable exactly-once
execution. The controller still needs an operation journal and status recovery
when a confirmation response is lost.

### rpcd Process Death: Automatic Recovery Failed

Sent SIGKILL to the verified rpcd process only. `procd` respawned it, PID `1187`
to `22461`; boot ID did not change. At t=75 seconds after apply, both committed
and runtime hostnames remained `wndr3800ch-lab`, past the 60-second deadline.

```text
OBSERVED rpcd restarted: PID 1187 -> 22461.
VERDICT at t=75s: checking original hostname without manual recovery.
FAIL hostname: expected=wndr3800ch configured=wndr3800ch-lab running=wndr3800ch-lab
```

Explicit recovery created a new rpcd session because the original was lost,
restored `wndr3800ch`, and confirmed the restoration. That repair is not evidence
of native automatic recovery. Gateway/other AP each answered 90/90 monitoring
pings during part of this experiment.

### Device Reboot: Automatic Recovery Failed

Issued exactly one `ubus call system reboot` after applying the temporary
hostname. The controller observed a new boot ID and available rpcd at t=234
seconds. Both committed and runtime hostnames still contained the unconfirmed
temporary value. No confirmation or manual rollback was sent before that verdict.

```text
OBSERVED new boot and rpcd available at t=234s.
VERDICT at t=234s: checking original hostname without manual recovery.
FAIL hostname: expected=wndr3800ch configured=wndr3800ch-lab running=wndr3800ch-lab
```

The observation includes reboot, startup, polling and RPC readiness; it is not a
precise boot-duration measurement. Hostname was then restored explicitly. The
network, DHCP, firewall, dropbear and system files matched the established
baseline. Wireless did not; see the independently discovered regression below.
Gateway and other AP each answered 120/120 monitoring pings, covering only part
of this longer experiment. Those probes do not establish end-user Internet health.

### Harness Limitations Encountered

The initial replay run expected a different CLI error text and stopped early;
the assertion was corrected for the observed `No response`. A later replay run
completed its observations but hit the host runner's 180-second limit during
cleanup. The original hostname had already been restored and no native rollback
was pending; the single empty lab staging session was removed separately.

Crash recovery initially ran in a subshell, losing the new session ID needed for
cleanup. That harness bug was corrected; its empty recovery session was removed
after verifying the restored hostname and absence of pending apply snapshots.
The corrected recovery path was exercised by the reboot experiment. These
harness issues do not change the pre-repair failure observations above.

## Wireless Regression Discovered on Reboot

Post-reboot verification found two WLAN interfaces in `sae-mixed`, whereas all
five were `psk2` in the preceding runtime check. Both radios were up. The live
drift endpoint reported `DRIFT_RELEVANT`, specifically expecting:

```text
uci set wireless.cfg_radio0_0.encryption='psk2'
```

Read-only controller projections confirmed contradictory desired values:

| Source | SSID | Security |
| --- | --- | --- |
| Site orchestrator template (`site-config`) | tuxcave2 | psk2 |
| WLAN profile (`wlans`, enabled, both bands, all devices) | tuxcave2 | sae-mixed |

The deployed `/usr/sbin/nerve-agent.sh` was inspected without exposing keys. It
uses `/tmp/wifi_config.hash` (line 453), reads each WLAN's `security` (491), writes
it to UCI encryption (512), commits wireless (556), then records the hash (558).
The corresponding repository implementation is in `devices/agent.sh:530-638`.
Provisioning reads WLAN rows, not the orchestrator encryption value, in
`internal/api/handlers/provision.go:174-208`. The volatile hash is lost on reboot,
allowing the WLAN profile to overwrite independently orchestrated values.

Final read-only checks after recovery: management LAN up at `10.128.128.3/24`,
original hostname restored, no global UCI changes or remaining lab artifacts,
controller active. Two wireless interfaces remain `sae-mixed`; the other three
are `psk2`. No forced wireless overwrite or shared WLAN-profile update was made.
Updating the shared profile could trigger automatic agent writes across the
site and was not part of this isolated test authorization.

This is a real configuration mismatch, not merely a byte-order difference. A
wireless hash difference was also noticed after crash recovery; without an
immediately captured pre-crash configuration diff its timing/content cannot be
attributed conclusively. The post-reboot encryption mismatch was measured directly.

## What This Does Not Establish

- All transport failure timings: the network experiment observed failed new SSH
  connections, not every failure timing of a still-running SSH command.
- Recovery after physical power loss, partial flash writes or storage failure.
- Durable task IDs, idempotent terminal results, or coordination with other writers.
- Cross-namespace atomicity or safety on other firmware/hardware.
- Runtime equivalence of every service: hashes validate files, not all runtime state.

Upstream source inspected as design evidence, not as proof of the installed
binary's behavior:

- https://github.com/openwrt/rpcd/blob/master/uci.c
- https://github.com/openwrt/rpcd/blob/master/include/rpcd/uci.h

In that source, the apply timer/session live in rpcd memory and snapshots are
under `/var/run/rpcd`. The process-death and reboot experiments now directly
demonstrate that native automatic recovery is insufficient on the tested node.
An independently supervised, persistent transaction journal and boot recovery
are required; native apply/confirm alone is not a complete safe executor.

Production network writes remain blocked pending a safe executor and broader
fault-injection coverage. No production Go code was changed by this experiment.
