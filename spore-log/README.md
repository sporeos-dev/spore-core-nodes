[← spore-core-nodes](../README.md)

# spore-log

Persistent rolling file logger for Spore OS hub traffic.

**Status:** Active development.

---

## How it works

`spore-log` is a witness node — it receives a copy of all hub traffic passively, without being the target of any call. It writes everything to a rolling log file, rotating at 10 MB.

Runs automatically on startup (`autostart: true`). No configuration required.

**Log location:**

| Platform | Path |
| :--- | :--- |
| macOS | `/Library/Logs/spore-os/spore.log` |

---

## License

Apache-2.0 — see [LICENSE](../LICENSE).
