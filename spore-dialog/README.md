[← spore-core-nodes](../README.md)

# spore-dialog

Native file and directory picker dialogs for Spore OS.

**Status:** Active development.

---

## How it works

`spore-dialog` is a helper node that exposes native OS dialogs as Spore subjects. Other nodes call it through the hub; it runs automatically on demand (`autostart: true`).

```
# pick a file
file.open ~h1
~h1:file.open path="/home/user/notes.txt" ok capture=dev.sporeos.dialog

# filter by extension
file.open ext=['.yaml'] ~h2
~h2:file.open path="/home/user/config.yaml" ok capture=dev.sporeos.dialog

# pick a directory
dir.open ~h3
~h3:dir.open path="/home/user/projects" ok capture=dev.sporeos.dialog

# choose a save path
file.save ~h4
~h4:file.save path="/home/user/output.txt" ok capture=dev.sporeos.dialog
```

If the user dismisses the dialog, the response is `cancelled` — not an error.

---

## License

Apache-2.0 — see [LICENSE](../LICENSE).
