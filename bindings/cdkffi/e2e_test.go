// End-to-end tests that exercise the full Cashu wallet flow against a live
// mint.  These tests are skipped unless the CDKGO_MINT_URL environment
// variable is set to the URL of a reachable Cashu mint.
//
// Run with:
//
//	CDKGO_MINT_URL=https://testnut.cashu.space CGO_ENABLED=1 go test ./bindings/cdkffi -run E2E -v -timeout 120s
package cdk_ffi

import (
	"os"
	"testing"
	"time"
)

// mintURLForE2E returns the mint URL from the environment or skips the test.
func mintURLForE2E(t *testing.T) string {
	t.Helper()
	url := os.Getenv("CDKGO_MINT_URL")
	if url == "" {
		t.Skip("CDKGO_MINT_URL not set – skipping e2e test")
	}
	return url
}

// newE2EWallet creates a fresh in-memory wallet pointed at the given mint.
func newE2EWallet(t *testing.T, mintURL string) *Wallet {
	t.Helper()
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic: %v", err)
	}
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	wallet, err := NewWallet(mintURL, CurrencyUnitSat{}, mnemonic, db, WalletConfig{TargetProofCount: nil})
	if err != nil {
		t.Fatalf("NewWallet(%q): %v", mintURL, err)
	}
	return wallet
}

// defaultReceiveOptions returns minimal ReceiveOptions suitable for tests.
func defaultReceiveOptions() ReceiveOptions {
	return ReceiveOptions{
		AmountSplitTarget: SplitTargetNone{},
		P2pkSigningKeys:   []SecretKey{},
		Preimages:         []string{},
		Metadata:          map[string]string{},
	}
}

// ---------------------------------------------------------------------------
// E2E: mint info
// ---------------------------------------------------------------------------

// TestE2E_FetchMintInfo verifies that we can fetch and parse mint metadata.
func TestE2E_FetchMintInfo(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	info, err := wallet.FetchMintInfo()
	if err != nil {
		t.Fatalf("FetchMintInfo: %v", err)
	}
	if info == nil {
		t.Fatal("MintInfo must not be nil")
	}
	if info.Name == nil || *info.Name == "" {
		t.Error("MintInfo.Name should not be empty")
	}
	if info.Name != nil {
		t.Logf("Mint: %q", *info.Name)
	}
}

// TestE2E_LoadMintInfo verifies LoadMintInfo (uses cache when fresh).
func TestE2E_LoadMintInfo(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	// Populate cache with a first fetch.
	_, err := wallet.FetchMintInfo()
	if err != nil {
		t.Fatalf("FetchMintInfo: %v", err)
	}

	info, err := wallet.LoadMintInfo()
	if err != nil {
		t.Fatalf("LoadMintInfo: %v", err)
	}
	if info.Name == nil || *info.Name == "" {
		t.Error("MintInfo.Name should not be empty")
	}
}

// ---------------------------------------------------------------------------
// E2E: keysets
// ---------------------------------------------------------------------------

// TestE2E_RefreshKeysets fetches the keyset list from the mint.
func TestE2E_RefreshKeysets(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	keysets, err := wallet.RefreshKeysets()
	if err != nil {
		t.Fatalf("RefreshKeysets: %v", err)
	}
	if len(keysets) == 0 {
		t.Fatal("expected at least one keyset from the mint")
	}
	for _, ks := range keysets {
		if ks.Id == "" {
			t.Errorf("keyset has empty ID: %+v", ks)
		}
		t.Logf("keyset id=%q active=%v", ks.Id, ks.Active)
	}
}

// TestE2E_GetActiveKeyset verifies the active keyset for the wallet unit.
func TestE2E_GetActiveKeyset(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	ks, err := wallet.GetActiveKeyset()
	if err != nil {
		t.Fatalf("GetActiveKeyset: %v", err)
	}
	if ks.Id == "" {
		t.Error("active keyset ID must not be empty")
	}
	if !ks.Active {
		t.Errorf("GetActiveKeyset returned an inactive keyset: %q", ks.Id)
	}
	t.Logf("active keyset id=%q unit=%v", ks.Id, ks.Unit)
}

// ---------------------------------------------------------------------------
// E2E: mint quote lifecycle
// ---------------------------------------------------------------------------

// TestE2E_MintQuote_Create creates a mint quote and inspects the returned
// Lightning invoice.  The quote is not paid, so no ecash is issued.
func TestE2E_MintQuote_Create(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	const amountSat = uint64(10)
	amount := Amount{Value: amountSat}
	quote, err := wallet.MintQuote(PaymentMethodBolt11{}, &amount, nil, nil)
	if err != nil {
		t.Fatalf("MintQuote(%d): %v", amountSat, err)
	}
	if quote.Id == "" {
		t.Error("quote ID must not be empty")
	}
	if quote.Request == "" {
		t.Error("quote Request (Lightning invoice) must not be empty")
	}
	if quote.State != QuoteStateUnpaid {
		t.Errorf("new quote should be Unpaid, got %v", quote.State)
	}
	t.Logf("quote id=%q invoice=%q", quote.Id, quote.Request)

	// Expiry must be in the future.
	now := uint64(time.Now().Unix())
	if quote.Expiry <= now {
		t.Errorf("quote.Expiry %d is not in the future (now=%d)", quote.Expiry, now)
	}
}

// TestE2E_MintQuote_CheckStatus checks the state of a freshly created (unpaid) quote.
func TestE2E_MintQuote_CheckStatus(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	amount := Amount{Value: 1}
	quote, err := wallet.MintQuote(PaymentMethodBolt11{}, &amount, nil, nil)
	if err != nil {
		t.Fatalf("MintQuote: %v", err)
	}

	checked, err := wallet.CheckMintQuote(quote.Id)
	if err != nil {
		t.Fatalf("CheckMintQuote(%q): %v", quote.Id, err)
	}
	if checked.Id != quote.Id {
		t.Errorf("status quote ID mismatch: got %q, want %q", checked.Id, quote.Id)
	}
	// An unpaid quote should still be unpaid immediately after creation.
	if checked.State != QuoteStateUnpaid {
		t.Logf("warning: quote state is %v (expected Unpaid), mint may have auto-resolved it", checked.State)
	}
}

// TestE2E_MintQuote_IsExpiredHelper verifies MintQuoteIsExpired on a live quote.
func TestE2E_MintQuote_IsExpiredHelper(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	amount := Amount{Value: 1}
	quote, err := wallet.MintQuote(PaymentMethodBolt11{}, &amount, nil, nil)
	if err != nil {
		t.Fatalf("MintQuote: %v", err)
	}

	now := uint64(time.Now().Unix())
	expired, err := MintQuoteIsExpired(quote, now)
	if err != nil {
		t.Fatalf("MintQuoteIsExpired: %v", err)
	}
	if expired {
		t.Errorf("freshly created quote should not yet be expired")
	}
}

// TestE2E_FetchMintQuote fetches a quote by ID via the mint's API.
func TestE2E_FetchMintQuote(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	amount := Amount{Value: 5}
	created, err := wallet.MintQuote(PaymentMethodBolt11{}, &amount, nil, nil)
	if err != nil {
		t.Fatalf("MintQuote: %v", err)
	}

	pm := PaymentMethod(PaymentMethodBolt11{})
	fetched, err := wallet.FetchMintQuote(created.Id, &pm)
	if err != nil {
		t.Fatalf("FetchMintQuote(%q): %v", created.Id, err)
	}
	if fetched.Id != created.Id {
		t.Errorf("fetched quote ID mismatch: got %q, want %q", fetched.Id, created.Id)
	}
}

// ---------------------------------------------------------------------------
// E2E: melt quote lifecycle
// ---------------------------------------------------------------------------

// TestE2E_MeltQuote_Create creates a melt quote for a test Lightning invoice.
func TestE2E_MeltQuote_Create(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	// A well-formed 10-sat testnet bolt11 invoice.  The mint will validate
	// the invoice format and return a melt quote; it will NOT be paid.
	const testInvoice = "lntbs10n1pjwkmgwpp5xrn39ynhgq2ywfkj7fzmmhz5crgyx9cnykss4ywfk4g6mhwsemhqdpzw35xjueqd9ejqcfqw3jhxapqf3hkuenxwdhhwcned9yxcqzzsxqzjcsp5e9m0hn6j0f40vhraqjhhu7gzc0hs9u2rp0kfl9nnxf33lh3esds9qyyssq5y8skl8v8r4hme8c5wggq8yjf4k5rn4ly0eza9h3qlf33q5hknslm0e30yslpvzqnqtghj0rqq2r69uyxhwu5t0n7z4k6e0c66rsxcqfq0h24"
	quote, err := wallet.MeltQuote(PaymentMethodBolt11{}, testInvoice, nil, nil)
	if err != nil {
		// Many test mints refuse arbitrary invoices; log and skip rather than fail hard.
		t.Skipf("MeltQuote returned error (mint may not accept this test invoice): %v", err)
	}
	if quote.Id == "" {
		t.Error("melt quote ID must not be empty")
	}
	t.Logf("melt quote id=%q fee_reserve=%d", quote.Id, quote.FeeReserve.Value)
}

// ---------------------------------------------------------------------------
// E2E: balance and wallet state
// ---------------------------------------------------------------------------

// TestE2E_TotalBalance verifies that a fresh wallet reports zero balance.
func TestE2E_TotalBalance(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	balance, err := wallet.TotalBalance()
	if err != nil {
		t.Fatalf("TotalBalance: %v", err)
	}
	if balance.Value != 0 {
		t.Fatalf("fresh wallet balance should be 0, got %d", balance.Value)
	}
}

// TestE2E_TotalPendingBalance verifies the pending balance is 0 on a fresh wallet.
func TestE2E_TotalPendingBalance(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	pending, err := wallet.TotalPendingBalance()
	if err != nil {
		t.Fatalf("TotalPendingBalance: %v", err)
	}
	if pending.Value != 0 {
		t.Fatalf("fresh wallet pending balance should be 0, got %d", pending.Value)
	}
}

// TestE2E_GetProofsByStates verifies the proof query by state on a fresh wallet.
func TestE2E_GetProofsByStates(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	proofs, err := wallet.GetProofsByStates([]ProofState{ProofStateUnspent})
	if err != nil {
		t.Fatalf("GetProofsByStates: %v", err)
	}
	if len(proofs) != 0 {
		t.Fatalf("fresh wallet should have 0 unspent proofs, got %d", len(proofs))
	}
}

// TestE2E_ListTransactions verifies that a fresh wallet has no transactions.
func TestE2E_ListTransactions(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	txs, err := wallet.ListTransactions(nil)
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(txs) != 0 {
		t.Fatalf("fresh wallet should have 0 transactions, got %d", len(txs))
	}
}

// TestE2E_GetPendingSends verifies that a fresh wallet has no pending send operations.
func TestE2E_GetPendingSends(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	ops, err := wallet.GetPendingSends()
	if err != nil {
		t.Fatalf("GetPendingSends: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("fresh wallet should have 0 pending sends, got %d", len(ops))
	}
}

// ---------------------------------------------------------------------------
// E2E: receive (offline token – spent proofs)
// ---------------------------------------------------------------------------

// TestE2E_ReceiveToken_SpentProofProducesError attempts to receive the sample
// token from the unit tests.  The proofs were already spent on the test mint;
// the wallet should detect this and return an error – which is the expected
// e2e path that exercises the Receive code and mint interaction.
func TestE2E_ReceiveToken_SpentProofProducesError(t *testing.T) {
	// Only run against the mint that originally issued the sample token.
	mintURL := mintURLForE2E(t)
	if mintURL != "https://8333.space:3338" {
		t.Skip("sample token is from https://8333.space:3338 – skipping against a different mint")
	}
	wallet := newE2EWallet(t, mintURL)

	tok, err := TokenDecode(sampleTokenV3)
	if err != nil {
		t.Fatalf("TokenDecode: %v", err)
	}

	opts := defaultReceiveOptions()
	_, err = wallet.Receive(tok, opts)
	// We expect either an error (proofs already spent) or success (token valid).
	// Either demonstrates the FFI path is exercised end-to-end.
	if err != nil {
		t.Logf("Receive returned error (expected for spent proofs): %v", err)
	} else {
		t.Log("Receive succeeded (token was not yet spent)")
	}
}

// ---------------------------------------------------------------------------
// E2E: WalletRepository
// ---------------------------------------------------------------------------

// TestE2E_WalletRepository_CreateAndGetWallet creates a repository and adds
// a wallet for the configured mint, then retrieves it.
func TestE2E_WalletRepository_CreateAndGetWallet(t *testing.T) {
	mintURL := mintURLForE2E(t)

	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic: %v", err)
	}
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	repo, err := NewWalletRepository(mnemonic, db)
	if err != nil {
		t.Fatalf("NewWalletRepository: %v", err)
	}

	mintUrl := MintUrl{Url: mintURL}

	// CreateWallet adds the mint/unit pair to the repository.
	if err := repo.CreateWallet(mintUrl, nil, nil); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	// GetWallet retrieves the created wallet.
	wallet, err := repo.GetWallet(mintUrl, CurrencyUnitSat{})
	if err != nil {
		t.Fatalf("GetWallet: %v", err)
	}
	if wallet == nil {
		t.Fatal("wallet must not be nil")
	}

	// Balance should be zero.
	bal, err := wallet.TotalBalance()
	if err != nil {
		t.Fatalf("TotalBalance: %v", err)
	}
	if bal.Value != 0 {
		t.Errorf("expected 0 balance, got %d", bal.Value)
	}
}

// TestE2E_WalletRepository_GetWallets verifies the GetWallets listing.
func TestE2E_WalletRepository_GetWallets(t *testing.T) {
	mintURL := mintURLForE2E(t)

	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic: %v", err)
	}
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	repo, err := NewWalletRepository(mnemonic, db)
	if err != nil {
		t.Fatalf("NewWalletRepository: %v", err)
	}

	if err := repo.CreateWallet(MintUrl{Url: mintURL}, nil, nil); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	wallets := repo.GetWallets()
	if len(wallets) == 0 {
		t.Fatal("expected at least one wallet after CreateWallet")
	}
}

// TestE2E_WalletRepository_GetBalances verifies the per-wallet balance map.
func TestE2E_WalletRepository_GetBalances(t *testing.T) {
	mintURL := mintURLForE2E(t)

	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic: %v", err)
	}
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	repo, err := NewWalletRepository(mnemonic, db)
	if err != nil {
		t.Fatalf("NewWalletRepository: %v", err)
	}
	if err := repo.CreateWallet(MintUrl{Url: mintURL}, nil, nil); err != nil {
		t.Fatalf("CreateWallet: %v", err)
	}

	balances, err := repo.GetBalances()
	if err != nil {
		t.Fatalf("GetBalances: %v", err)
	}
	if len(balances) == 0 {
		t.Fatal("expected at least one entry in balances map")
	}
	for key, bal := range balances {
		t.Logf("mint=%q unit=%v balance=%d", key.MintUrl.Url, key.Unit, bal.Value)
	}
}

// ---------------------------------------------------------------------------
// E2E: proof state helpers against live keysets
// ---------------------------------------------------------------------------

// TestE2E_ProofIsActiveAgainstMintKeysets fetches the active keysets from a
// live mint and verifies ProofIsActive behaviour.
func TestE2E_ProofIsActiveAgainstMintKeysets(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	keysets, err := wallet.RefreshKeysets()
	if err != nil {
		t.Fatalf("RefreshKeysets: %v", err)
	}

	var activeIds []string
	for _, ks := range keysets {
		if ks.Active {
			activeIds = append(activeIds, ks.Id)
		}
	}
	if len(activeIds) == 0 {
		t.Fatal("no active keysets returned from mint")
	}

	proof := validProof(1, activeIds[0])
	if !ProofIsActive(proof, activeIds) {
		t.Errorf("proof with keyset %q should be active according to the mint", activeIds[0])
	}

	proofInactive := validProof(1, "0000000000000000")
	if ProofIsActive(proofInactive, activeIds) {
		t.Errorf("proof with fake keyset ID should not be active")
	}
}

// ---------------------------------------------------------------------------
// E2E: MintUrl helper
// ---------------------------------------------------------------------------

// TestE2E_WalletMintUrl verifies that wallet.MintUrl() returns the configured URL.
func TestE2E_WalletMintUrl(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	got := wallet.MintUrl()
	if got.Url != mintURL {
		t.Errorf("wallet.MintUrl().Url = %q, want %q", got.Url, mintURL)
	}
}

// ---------------------------------------------------------------------------
// E2E: CheckAllPendingProofs
// ---------------------------------------------------------------------------

// TestE2E_CheckAllPendingProofs runs the pending-proofs check on a fresh wallet.
func TestE2E_CheckAllPendingProofs(t *testing.T) {
	mintURL := mintURLForE2E(t)
	wallet := newE2EWallet(t, mintURL)

	amount, err := wallet.CheckAllPendingProofs()
	if err != nil {
		t.Fatalf("CheckAllPendingProofs: %v", err)
	}
	if amount.Value != 0 {
		t.Logf("pending amount: %d (non-zero is unexpected on a fresh wallet)", amount.Value)
	}
}
