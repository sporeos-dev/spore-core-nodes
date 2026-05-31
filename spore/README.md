[← spore-core-nodes](../README.md)

# spore

One-off command-line tool for Spore OS. Sends a single command to the hub and prints the response.

**Status:** Active development.

---

## How it works

`spore` connects to the hub, sends one message, prints the response, and exits. Handles are auto-generated if not supplied.

```sh
spore file.open
# ~s1a2b:file.open path="/home/user/notes.txt" ok capture=dev.sporeos.dialog

spore SPORE.node.list
# ~s3f4c:SPORE.node.list ... ok capture=SPORE.hub
```

Running `spore` with no arguments drops into `spore-shell`.

---

## License

Apache-2.0 — see [LICENSE](../LICENSE).
