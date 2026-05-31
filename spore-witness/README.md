[← spore-core-nodes](../README.md)

# spore-witness

Live colour-coded hub traffic viewer for active debugging.

**Status:** Active development.

---

## How it works

`spore-witness` is a witness node — it receives a copy of all hub traffic passively and prints it to stdout with colour-coded labels. Run it manually when debugging; it does not autostart.

```sh
spore-witness
# dev.sporeos.shell → file.open ~h1
# dev.sporeos.dialog → ~h1:file.open path="/home/user/notes.txt" ok
```

---

## License

Apache-2.0 — see [LICENSE](../LICENSE).
