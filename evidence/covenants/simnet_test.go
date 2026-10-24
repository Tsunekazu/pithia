package covenants

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/decred/dcrd/chaincfg/v3"
	"github.com/decred/dcrd/txscript/v4/stdaddr"
	"github.com/decred/dcrd/wire"
)

// Explicit opt-in only. This starts a fresh private simulation chain, not a
// wallet or a public-network connection. RPC credentials are public test data.
func TestSimnetFundedCovenant(t *testing.T) {
	bin := os.Getenv("COVENANT_SIMNET_DCRD")
	if bin == "" {
		t.Skip("set COVENANT_SIMNET_DCRD to an official dcrd binary")
	}
	root, err := os.MkdirTemp("/tmp", "pi-ex-covenant-simnet-")
	if err != nil {
		t.Fatal(err)
	}
	const fixedOutput = 499990000 // Known 5-DCR simnet mining reward less10000atoms.
	script := covenantScriptAmount(fixedOutput)
	addr, err := stdaddr.NewAddressScriptHashV0(script, chaincfg.SimNetParams())
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
	args := []string{"--simnet", "--appdata=" + root, "--datadir=" + filepath.Join(root, "data"), "--logdir=" + filepath.Join(root, "logs"), "--nolisten", "--nodnsseed", "--maxpeers=0", "--rejectnonstd", "--minrelaytxfee=0.0001", "--notls", "--rpclisten=" + fmt.Sprintf("127.0.0.1:%d", port), "--rpcuser=covenant-local-test", "--rpcpass=public-simulation-only", "--miningaddr=" + addr.String(), "--txindex"}
	log, err := os.Create(filepath.Join(root, "console.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.Command(bin, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	rpc := func(method string, params []any) (json.RawMessage, error) {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": params})
		req, _ := http.NewRequest("POST", endpoint, bytes.NewReader(body))
		req.SetBasicAuth("covenant-local-test", "public-simulation-only")
		req.Header.Set("Content-Type", "application/json")
		resp, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		var out struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if e = json.NewDecoder(resp.Body).Decode(&out); e != nil {
			return nil, e
		}
		if out.Error != nil {
			return nil, fmt.Errorf("RPC %d: %s", out.Error.Code, out.Error.Message)
		}
		return out.Result, nil
	}
	defer func() {
		rpc("stop", nil)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	}()
	started := false
	for i := 0; i < 100; i++ {
		if _, e := rpc("getblockcount", nil); e == nil {
			started = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !started {
		b, _ := os.ReadFile(filepath.Join(root, "console.log"))
		t.Fatalf("node did not start: %s", b)
	}
	must := func(method string, params []any) json.RawMessage {
		r, e := rpc(method, params)
		if e != nil {
			t.Fatalf("%s: %v (logs %s)", method, e, root)
		}
		return r
	}
	connections := must("getconnectioncount", nil)
	if string(connections) != "0" {
		t.Fatal("unexpected peers:", string(connections))
	}
	var blocks []string
	if err = json.Unmarshal(must("generate", []any{32}), &blocks); err != nil {
		t.Fatal(err)
	}
	var blockhex string
	json.Unmarshal(must("getblock", []any{blocks[1], false}), &blockhex)
	raw, _ := hex.DecodeString(blockhex)
	var block wire.MsgBlock
	if err = block.Deserialize(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	version, payScript := addr.PaymentScript()
	if version != 0 {
		t.Fatal("unexpected funding version")
	}
	coinbase := block.Transactions[0]
	idx := -1
	for i, o := range coinbase.TxOut {
		if bytes.Equal(o.PkScript, payScript) {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("coinbase did not pay covenant P2SH")
	}
	amount := coinbase.TxOut[idx].Value
	if amount != 500000000 {
		t.Fatalf("known-value experiment requires500000000atoms, got%d", amount)
	}
	tx := wire.NewMsgTx()
	tx.Version = 2
	hash := coinbase.TxHash()
	tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(&hash, uint32(idx), wire.TxTreeRegular), amount, nil))
	tx.TxIn[0].BlockHeight = 2
	tx.TxIn[0].BlockIndex = 0
	tx.AddTxOut(wire.NewTxOut(fixedOutput, outputScript()))
	grind(t, tx, script)
	// The negative must fail script checks while the UTXO is still unspent.
	bad := tx.Copy()
	bad.TxOut[0].PkScript[3] ^= 1
	grind(t, bad, script)
	bp, bw := claims(bad, script)
	_ = run(bad, script, bp, bw)
	badraw, _ := bad.Bytes()
	_, reject := rpc("sendrawtransaction", []any{hex.EncodeToString(badraw), false})
	if reject == nil {
		t.Fatal("forbidden output admitted by full node")
	}
	if !bytes.Contains([]byte(reject.Error()), []byte("OP_EQUALVERIFY")) {
		t.Fatalf("negative failed for a different reason: %v", reject)
	}
	t.Log("full node rejected changed destination:", reject)
	p, w := claims(tx, script)
	if err = run(tx, script, p, w); err != nil {
		t.Fatal(err)
	}
	txraw, _ := tx.Bytes()
	if amount-fixedOutput < int64(len(txraw))*10000/1000 {
		t.Fatal("fee below configured relay rate")
	}
	var txid string
	json.Unmarshal(must("sendrawtransaction", []any{hex.EncodeToString(txraw), false}), &txid)
	var pool []string
	json.Unmarshal(must("getrawmempool", nil), &pool)
	found := false
	for _, h := range pool {
		if h == txid {
			found = true
		}
	}
	if !found {
		t.Fatal("accepted tx absent from mempool")
	}
	// Mining uses a background template cache; explicitly refresh it after
	// mempool insertion rather than accidentally mining its old empty template.
	must("regentemplate", nil)
	time.Sleep(1200 * time.Millisecond)
	var mined []string
	json.Unmarshal(must("generate", []any{1}), &mined)
	json.Unmarshal(must("getblock", []any{mined[0], false}), &blockhex)
	raw, _ = hex.DecodeString(blockhex)
	var included wire.MsgBlock
	if err = included.Deserialize(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	found = false
	for _, tr := range included.Transactions {
		if tr.TxHash().String() == txid {
			found = true
		}
	}
	if !found {
		t.Fatal("covenant spend absent from mined block")
	}
	t.Logf("PASS actual simnet UTXO: coinbase height2, amount=%d atoms, fixed output=%d, fee=%d, txbytes=%d, allowhighfees=false; txid=%s", amount, fixedOutput, amount-fixedOutput, len(txraw), txid)
	t.Logf("PASS --rejectnonstd mempool admission and block inclusion at height33; zero peers; node directory %s", root)
	ver, _ := exec.Command(bin, "--version").Output()
	report := map[string]any{"node_version": string(bytes.TrimSpace(ver)), "network": "simnet", "peers": 0, "reject_nonstandard": true, "funding_height": 2, "spend_height": 33, "funding_atoms": amount, "output_atoms": fixedOutput, "fee_atoms": amount - fixedOutput, "transaction_bytes": len(txraw), "relay_fee_atoms_per_kb": 10000, "high_fee_rpc_override": false, "txid": txid, "mined_block": mined[0], "negative_destination_error": reject.Error(), "node_directory": root, "node_flags": args}
	j, _ := json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile("simnet-v2.1.7-result.json", append(j, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
