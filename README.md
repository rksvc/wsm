[NSSM](https://nssm.cc)-inspired Windows service manager with native GUI.

## Build

```sh
rsrc -manifest wsm.manifest -o rsrc.syso
go build -ldflags='-s -w -H windowsgui' -trimpath
```
