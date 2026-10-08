# ADR 0004: Fenced leases and bounded draining

Status: accepted

Workers renew leases while executing. A fencing token prevents an expired worker from overwriting a newer result. On shutdown a worker stops consuming, advertises `draining`, and continues active lease renewal for a bounded period. At the deadline it atomically releases unfinished jobs for retry. This favors availability without allowing deployments to hang indefinitely.
