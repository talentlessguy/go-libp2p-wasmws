# go-libp2p-wasmws

A go-libp2p dial-only WebSocket transport that works with WebAssembly.

## Install

```sh
go get github.com/talentlessguy/go-libp2p-wasmws
```

## Usage

```go
// go build: js || wasm
//go:build js || wasm

package main

import (
        "log"

        "github.com/libp2p/go-libp2p"
        wasmws "github.com/talentlessguy/go-libp2p-wasmws"

        "github.com/libp2p/go-libp2p/p2p/security/noise"
)

func main() {
        host, err := libp2p.New(
                libp2p.Transport(wasmws.New),
                libp2p.Security(noise.ID, noise.New),
        )
        if err != nil {
                log.Fatalf("failed to create libp2p host: %v", err)
        }
        defer host.Close()
}
```