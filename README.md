# Pithia

Independent research and experiments for Decred. This is a concept preview.

**Live page:** https://tsunekazu.github.io/pithia/

Pithia investigates what Decred could build, tests the promising ideas and publishes the evidence. Stakeholders vote on what comes first. AI agents do the digging and prototyping; a human editor reviews the evidence and marks what is still unresolved.

The page is plain HTML and CSS: no JavaScript, no trackers, no third-party requests. The vote and signal screens on it are demos and record nothing.

## Reproduce the covenant experiment

Investigation 01 is a 196-byte covenant built from opcodes Decred already has. dcrd's own script engine accepts it, and a real spend was mined on a private simnet. To run it yourself:

```sh
cd evidence/covenants
go test -count=1 -v ./...                       # engine checks, txscript v4.1.2 (Go 1.24+)
COVENANT_SIMNET_DCRD=/path/to/dcrd \
  go test -count=1 -run TestSimnetFundedCovenant -v ./...   # full node on a private simnet, dcrd 2.1.7
```

[`README.md`](evidence/covenants/README.md) and [`SIMNET.md`](evidence/covenants/SIMNET.md) explain what the experiment shows, and what it does not. It is not a vault or a security audit.

## Contact

Bison Relay `tivra` · Telegram [@tivra](https://t.me/tivra) · X [@MercurialPraxis](https://x.com/MercurialPraxis)

## Credits

An independent concept by Jure, not an official Decred project. The investigations were produced with AI agents under Jure's direction. Illustrations were made with an image model and dithered to 1-bit by script.

Fonts: IBM Plex and VT323, both under the SIL Open Font License 1.1. The licence texts are in [`fonts/`](fonts/).
