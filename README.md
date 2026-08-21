A [NSSM](https://nssm.cc)-inspired Windows service manager with a Web-based UI.

It can display services installed with NSSM in the UI but cannot edit them, because this software is not compatible with NSSM.

## Usage

```
Usage of wsm.exe:
  -addr string
        address to run server on (default "127.0.0.1:3483")
```

## Build

```sh
pnpm install
pnpm build
go build -ldflags='-s -w' -trimpath
```

## Credits

- [fluentui-emoji](https://github.com/microsoft/fluentui-emoji) for favicon.
