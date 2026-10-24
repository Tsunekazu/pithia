package covenants

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/crypto/blake256"
	"github.com/decred/dcrd/crypto/ripemd160"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/txscript/v4"
	"github.com/decred/dcrd/wire"
)

// Research only: a fixed-output, one-input covenant using only existing version
// zero opcodes. It is NOT a vault, wallet, deployment or security-reviewed code.
var generatorX, _ = hex.DecodeString("79be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")

func outputScript() []byte {
	return append(append([]byte{txscript.OP_DUP, txscript.OP_HASH160, 20}, bytes.Repeat([]byte{0x42}, 20)...), txscript.OP_EQUALVERIFY, txscript.OP_CHECKSIG)
}

func covenantScript() []byte {
	return covenantScriptAmount(90000)
}

func covenantScriptAmount(amount uint64) []byte {
	out := make([]byte, 0, 37)
	out = append(out, 1) // exactly one output
	value := make([]byte, 8)
	binary.LittleEndian.PutUint64(value, amount)
	out = append(out, value...)
	out = append(out, 0, 0, 25)
	out = append(out, outputScript()...)
	negG := append([]byte{3}, generatorX...)
	b := txscript.NewScriptBuilder()
	// Input stack: <claimed prefix serialization> <claimed witness hash>.
	b.AddOp(txscript.OP_SIZE).AddInt64(32).AddOp(txscript.OP_NUMEQUALVERIFY).AddOp(txscript.OP_SWAP)
	b.AddOp(txscript.OP_SIZE).AddInt64(91).AddOp(txscript.OP_NUMEQUALVERIFY)
	// Pin version 2, sighash prefix serialization type 1, one input.
	b.AddOp(txscript.OP_DUP).AddInt64(5).AddOp(txscript.OP_LEFT).AddData([]byte{2, 0, 1, 0, 1}).AddOp(txscript.OP_EQUALVERIFY)
	// Pin one output and its complete amount, version and destination script.
	b.AddOp(txscript.OP_DUP).AddInt64(83).AddOp(txscript.OP_LEFT).AddInt64(46).AddOp(txscript.OP_RIGHT).AddData(out).AddOp(txscript.OP_EQUALVERIFY)
	// Build the exact native SigHashAll digest from the constrained prefix.
	b.AddOp(txscript.OP_BLAKE256).AddData([]byte{1, 0, 0, 0}).AddOp(txscript.OP_SWAP).AddOp(txscript.OP_CAT).AddOp(txscript.OP_SWAP).AddOp(txscript.OP_CAT).AddOp(txscript.OP_BLAKE256)
	// For Q=-G,R=G, native Schnorr s=1+e, e=BLAKE256(R.x||m).
	b.AddData(generatorX).AddOp(txscript.OP_SWAP).AddOp(txscript.OP_CAT).AddOp(txscript.OP_BLAKE256)
	// Require final e byte=01, so incrementing it to02 has no carry.
	b.AddOp(txscript.OP_DUP).AddInt64(31).AddOp(txscript.OP_RIGHT).AddData([]byte{1}).AddOp(txscript.OP_EQUALVERIFY)
	b.AddInt64(31).AddOp(txscript.OP_LEFT).AddData([]byte{2}).AddOp(txscript.OP_CAT)
	// Synthesize (R.x,s) and append SIGHASH_ALL, then native verification.
	b.AddData(generatorX).AddOp(txscript.OP_SWAP).AddOp(txscript.OP_CAT).AddData([]byte{1}).AddOp(txscript.OP_CAT)
	b.AddData(negG).AddInt64(int64(dcrec.STSchnorrSecp256k1)).AddOp(txscript.OP_CHECKSIGALT)
	s, err := b.Script()
	if err != nil {
		panic(err)
	}
	return s
}

func witnessHash(tx *wire.MsgTx, script []byte) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint32(tx.Version)|uint32(txscript.SigHashSerializeWitness)<<16)
	wire.WriteVarInt(&b, 0, 1)
	wire.WriteVarBytes(&b, 0, script)
	h := chainhash.HashH(b.Bytes())
	return h[:]
}

func claims(tx *wire.MsgTx, script []byte) ([]byte, []byte) {
	p, err := tx.BytesPrefix()
	if err != nil {
		panic(err)
	}
	return p, witnessHash(tx, script)
}

func grind(t *testing.T, tx *wire.MsgTx, script []byte) {
	t.Helper()
	for i := uint32(0); i < 65536; i++ {
		tx.LockTime = i
		h, err := txscript.CalcSignatureHash(script, txscript.SigHashAll, tx, 0, nil)
		if err != nil {
			t.Fatal(err)
		}
		e := blake256.Sum256(append(append([]byte{}, generatorX...), h...))
		if e[31] == 1 {
			return
		}
	}
	t.Fatal("no valid grinding result in bounded locktime range")
}

func run(tx *wire.MsgTx, script, p, w []byte) error {
	// Standard P2SH funding wrapper; the covenant is its redeem script.
	h := blake256.Sum256(script)
	rh := ripemd160.New()
	rh.Write(h[:])
	pkScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_HASH160).AddData(rh.Sum(nil)).AddOp(txscript.OP_EQUAL).Script()
	if err != nil {
		return err
	}
	sig, err := txscript.NewScriptBuilder().AddData(p).AddData(w).AddData(script).Script()
	if err != nil {
		return err
	}
	tx.TxIn[0].SignatureScript = sig
	if len(sig) > 1650 {
		return fmt.Errorf("exceeds standard signature-script size: %d", len(sig))
	}
	vm, err := txscript.NewEngine(pkScript, tx, 0, txscript.ScriptVerifyCleanStack|txscript.ScriptVerifySigPushOnly, 0, nil)
	if err != nil {
		return err
	}
	return vm.Execute()
}

func TestNativeFixedOutputCovenant(t *testing.T) {
	script := covenantScript()
	tx := wire.NewMsgTx()
	tx.Version = 2
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.Hash{1}, Index: 0, Tree: wire.TxTreeRegular}, 100000, nil))
	tx.AddTxOut(wire.NewTxOut(90000, outputScript()))
	grind(t, tx, script)
	p, w := claims(tx, script)
	if err := run(tx, script, p, w); err != nil {
		t.Fatal("valid constrained payment:", err)
	}
	tokenizer := txscript.MakeScriptTokenizer(0, script)
	ops := 0
	for tokenizer.Next() {
		if tokenizer.Opcode() > txscript.OP_16 {
			ops++
		}
	}
	if tokenizer.Err() != nil {
		t.Fatal(tokenizer.Err())
	}
	t.Logf("PASS P2SH native covenant; redeem=%d bytes, sigscript=%d bytes, nonpush_ops=%d, sigops=%d, prefix=%d bytes, grinding locktime=%d", len(script), len(tx.TxIn[0].SignatureScript), ops, txscript.GetSigOpCount(script, false), len(p), tx.LockTime)
	alternate := tx.Copy()
	alternate.TxIn[0].PreviousOutPoint.Hash[0] ^= 0x55
	grind(t, alternate, script)
	ap, aw := claims(alternate, script)
	if err := run(alternate, script, ap, aw); err != nil {
		t.Fatal("same covenant must support different deposit outpoint:", err)
	}
	t.Log("PASS same covenant on distinct deposit outpoint without an external signature")
	for _, what := range []string{"destination", "amount", "extra_output", "wrong_prefix", "wrong_witness"} {
		t.Run(what, func(t *testing.T) {
			changed := tx.Copy()
			cp, cw := append([]byte{}, p...), append([]byte{}, w...)
			switch what {
			case "destination":
				changed.TxOut[0].PkScript[3] ^= 1
			case "amount":
				changed.TxOut[0].Value--
			case "extra_output":
				changed.AddTxOut(wire.NewTxOut(1, []byte{txscript.OP_TRUE}))
			case "wrong_prefix":
				cp[5] ^= 1
			case "wrong_witness":
				cw[0] ^= 1
			}
			if err := run(changed, script, cp, cw); err == nil {
				t.Fatal("forbidden change accepted with original claims")
			}
			if what == "destination" || what == "amount" || what == "extra_output" {
				grind(t, changed, script)
				cp, cw = claims(changed, script)
				if err := run(changed, script, cp, cw); err == nil {
					t.Fatal("forbidden output accepted with fresh true claims")
				}
			}
			t.Log("rejected as required")
		})
	}
	fmt.Printf("Native Decred fixed-output covenant engine checks passed; NOT a production vault or relay/chain validation.\n")
}
