# Native Decred fixed-output covenant experiment

**Result, 2026-10-24: the official Decred script engine accepts a small P2SH
covenant that requires one exact output, without an external transaction
signature or a changed opcode.** This is an offline research result. It is not a
vault, wallet, novel-cryptography claim, security audit, or deployment approval.

The experiment uses `github.com/decred/dcrd/txscript/v4 v4.1.2`, pinned together
with its dependencies in `go.mod` and `go.sum`. It does not modify dependency
source. No node, wallet, funds, address belonging to a user, or live chain was
used. All input outpoints are synthetic. The test operates only on ordinary
transaction scripts; it establishes nothing about staking tickets or treasury
spends.

## What it establishes

The 196-byte redeem script enforces a canonical, one-input, one-output version-2
transaction prefix. Its complete output is fixed: amount, script version, and
P2PKH destination. The script constructs the native signature digest from that
constrained prefix and a supplied witness hash, then checks a Schnorr signature
that the script itself synthesizes.

The native Decred verifier uses `e = BLAKE256(r || m)` and verifies
`R = sG + eQ`. Choosing `Q = -G`, `R = G` makes `s = 1 + e`. Grinding the
transaction's bounded locktime until the final byte of `e` is `01` allows the
script to construct `s` by replacing that byte with `02`, without implementing
256-bit arithmetic. The script forces both the Schnorr suite and `SIGHASH_ALL`;
the caller cannot supply an alternative signature.

The native signature check binds the supplied prefix and witness hash to the
actual transaction digest, under the usual hash/signature assumptions. The
script need not embed its own hash: the witness hash can be supplied because it
is bound by this check. The same covenant also passes for a distinct synthetic
deposit outpoint, so it is not merely a signature for one preselected transaction.

Five negative subtests reject changed destination, changed amount, extra output,
false prefix, and false witness hash. Output mutations are checked both with the
original claims and with freshly computed, re-ground true claims. Thus those
negative cases check actual output restrictions, not only consistency of a
caller-supplied preimage.

Captured output is in `result.txt`: redeem script 196 bytes, signature script 324
bytes, 30 non-push operations, one signature operation. The engine runs with
clean-stack and push-only-signature-script flags. The funding wrapper has the
standard P2SH shape, and the signature script is below the sourced 1650-byte
policy bound. **These observations are not a complete mempool policy test.**

## Reproduce

An official Go 1.27.3 archive was downloaded into `/tmp/pi-ex-covenant-go` and
SHA256-checked against official `https://go.dev/dl/?mode=json` metadata:

`65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40`

The archive and toolchain receipt remain under `/tmp`, outside this experiment.
No system toolchain was installed. Run from this directory:

```sh
env GOPATH=/tmp/pi-ex-covenant-go/path \
    GOCACHE=/tmp/pi-ex-covenant-go/cache GOTOOLCHAIN=local \
    /tmp/pi-ex-covenant-go/go/bin/go test -count=1 -v ./...
```

## Limits and next useful experiment

This does not execute dcrd contextual transaction checks, a real UTXO spend,
mempool admission, block inclusion, or a multi-transaction vault lifecycle. The
script is an intentionally fixed payment predicate, not a general compiler.
It has no authorization policy beyond the output restriction. A caller who can
provide the public data can execute the permitted payment.

Decred's input `ValueIn` is not included in this signature digest. Consequently,
this result does not establish an arbitrary-balance covenant that preserves the
entire input value. Fixed-denomination deposits are a narrower possible starting
point; more general value handling requires a separate construction and review.

The next bounded step is a fixed-denomination, nonrecursive two-stage vault on a
local simulation chain: hot authorization can only create a staging output;
CSV delays its final withdrawal; cold authorization can recover during the
delay. That requires proving the complete lifecycle and fee behavior, testing
real policy/contextual checks, and independent adversarial review. Monitoring,
cold-signing UX, fee/pinning resilience, and any staking integration remain
separate work. No claim of first discovery is made: a bounded search found no
Decred-specific implementation, which is not an exhaustive prior-art search.

## Primary design and implementation sources

- [Poelstra, CAT and Schnorr Tricks I (2021)](https://www.wpsoftware.net/andrew/blog/cat-and-schnorr-tricks-i.html)
- [Poelstra, CAT and Schnorr Tricks II, including vaults (2021)](https://www.wpsoftware.net/andrew/blog/cat-and-schnorr-tricks-ii.html)
- [Decred native opcode implementation](https://github.com/decred/dcrd/blob/master/txscript/opcode.go)
- [Decred native signature-hash implementation](https://github.com/decred/dcrd/blob/master/txscript/sighash.go)
- [Decred EC-Schnorr-DCRv0 verifier](https://github.com/decred/dcrd/blob/master/dcrec/secp256k1/schnorr/signature.go)
- [Decred mempool policy](https://github.com/decred/dcrd/blob/master/internal/mempool/policy.go)

The Bitcoin designs motivate the construction. Decred's Schnorr convention,
hash function, transaction serialization and script behavior were adapted
explicitly; Bitcoin script code was not presumed to port unchanged.
