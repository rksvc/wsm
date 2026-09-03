[NSSM](https://nssm.cc)-inspired Windows service manager with native GUI.

It can display services installed with NSSM in the UI but cannot edit them, because this software is not compatible with NSSM.

## Build

```sh
rsrc -manifest wsm.manifest -o rsrc.syso
go build -ldflags='-s -w -H windowsgui' -trimpath
```
