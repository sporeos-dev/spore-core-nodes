[← spore-core-nodes](../README.md)

# spore-shell

Interactive shell (REPL) for sending commands to the Spore OS hub.

**Status:** Active development.

---

## How it works

`spore-shell` maintains a persistent connection to the hub. Commands are sent as you type; responses print above the prompt without interrupting your input.

```
$ spore-shell
> file.open
~h1:file.open path="/home/user/notes.txt" ok capture=dev.sporeos.dialog
> SPORE.node.list
~h2:SPORE.node.list ...
```

Also exposes an `echo` subject for testing:

```
> echo expression="hello world"
~h3:echo echo="hello world" ok capture=dev.sporeos.shell
```

---

## License

Apache-2.0 — see [LICENSE](../LICENSE).
