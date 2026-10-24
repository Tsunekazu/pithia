# Funded covenant check on isolated simnet

**2026-10-24: PASS on official dcrd release-v2.1.7.** A real UTXO in a fresh
private simulation chain was spent through the native fixed-output P2SH
covenant, admitted to the node's mempool with nonstandard transactions rejected,
then included in a mined block. A changed destination was rejected specifically
by `OP_EQUALVERIFY` while that UTXO was still unspent.

Evidence:

- `simnet_test.go`: opt-in integration test; starts and stops its own private node.
- `simnet-v2.1.7-result.txt`: captured successful test output.
- `simnet-v2.1.7-result.json`: amounts, transaction/block IDs, flags, and node path.
- `node-v2.1.7-build.txt`: Go build/module checksum receipt for the node binary.

The official release tag resolves to commit
`e07b56553d6d6af84aa139e33b4f6368c55c9d2d`, dated 2026-10-13. Go resolves it to
module version `v1.10.9`; the binary reports `2.1.7+release.local`. Public module
downloads used Go's checksum verification. The binary was built into `/tmp`,
with the previously SHA256-verified official Go toolchain; no system software
was installed.

## Exact check

The node ran with `--simnet --nolisten --nodnsseed --maxpeers=0 --rejectnonstd`,
loopback-only RPC, fresh data directories under `/tmp`, and an explicit
`--minrelaytxfee=0.0001` DCR/kB. The test checked zero peers. It used public dummy
RPC credentials, no wallet, no private signing key, and no public-chain funds.
The node shut down through its RPC and the test waited for process exit.

Mining paid a P2SH address for the covenant. After mining 32 blocks, the test
spent the mature mining output from height 2:

| Property | Value |
|---|---|
| Funding | 500000000 atoms (5 simulated DCR) |
| Required output | 499990000 atoms |
| Fee | 10000 atoms (0.0001 simulated DCR) |
| Serialized transaction | 435 bytes |
| High-fee RPC override | false |
| Mined spend | height 33 |

The output amount is selected **before constructing this covenant**, from the
known simulation mining reward. The test asserts that the actual funding amount
is exactly that expected value. This is not an arbitrary-balance covenant and
does not infer input-value introspection. A different fixed amount creates a
different redeem script/address.

After mempool insertion, the test explicitly refreshed the background mining
template before mining; otherwise the first cached template could omit the
new transaction. The initial attempt encountered that cached-template behavior;
it did not establish a covenant validation failure.

## What remains open

This checks actual simnet UTXO/contextual validation, current-node mempool
policy with standardness required, and mined inclusion. It does not prove the
entire mainnet execution environment is identical to a height-33 simulation
chain, nor provide a security audit. No public network was contacted by dcrd.

The permitted payment is still permissionless to trigger and fixed in advance.
The test does not implement hot-key authorization, staging outputs, CSV
withdrawal delay, cold recovery, flexible fees, safe overfunding handling,
staking protection, or a wallet. Those remain separate constructions and
acceptance targets. This result strengthens the primitive evidence; it does not
turn the primitive into a useful or production-ready savings vault.

The earlier `simnet-result.txt` and `simnet-result.json` are preserved historical
evidence from dcrd 2.1.6 with an intentionally excessive simulated fee and a
high-fee RPC override. They are superseded by the 2.1.7 check for current-version
and ordinary-fee claims. The original `README.md` describes the earlier
engine-only result and its evidence level; this file records the later advance.

## Reproduce

From this directory, with the official binary built in `/tmp`:

```sh
env GOPATH=/tmp/pi-ex-covenant-go/path \
    GOCACHE=/tmp/pi-ex-covenant-go/cache GOTOOLCHAIN=local \
    COVENANT_SIMNET_DCRD=/tmp/pi-ex-covenant-go/bin/dcrd \
    /tmp/pi-ex-covenant-go/go/bin/go test -count=1 \
    -run TestSimnetFundedCovenant -v ./...
```

The opt-in test needs permission to bind a loopback RPC socket. Without that
environment variable, normal `go test` runs the original engine checks and skips
the full-node check.
