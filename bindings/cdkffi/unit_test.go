// Package cdk_ffi provides Go bindings for the Cashu Development Kit (CDK).
// This file contains unit tests that exercise the Go–Rust FFI layer without
// requiring a live Cashu mint.  Every test here must pass with only the
// committed prebuilt native libraries (no network, no external services).
package cdk_ffi

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// mustMnemonic generates a fresh BIP-39 mnemonic and fails the test on error.
func mustMnemonic(t *testing.T) string {
	t.Helper()
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic: %v", err)
	}
	return mnemonic
}

// ---------------------------------------------------------------------------
// Mnemonic / entropy
// ---------------------------------------------------------------------------

func TestGenerateMnemonic_ReturnsNonEmpty(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mnemonic == "" {
		t.Fatal("mnemonic must not be empty")
	}
}

func TestGenerateMnemonic_ReturnsTwelveOrTwentyFourWords(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	words := strings.Fields(mnemonic)
	if len(words) != 12 && len(words) != 24 {
		t.Fatalf("expected 12 or 24 words, got %d (%q)", len(words), mnemonic)
	}
}

func TestGenerateMnemonic_UniqueOnEachCall(t *testing.T) {
	a, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	b, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if a == b {
		t.Fatalf("two sequential GenerateMnemonic calls returned identical output: %q", a)
	}
}

func TestMnemonicToEntropy_RoundTrip(t *testing.T) {
	mnemonic := mustMnemonic(t)
	entropy, err := MnemonicToEntropy(mnemonic)
	if err != nil {
		t.Fatalf("MnemonicToEntropy: %v", err)
	}
	if len(entropy) == 0 {
		t.Fatal("entropy must not be empty")
	}
	// Entropy must be 16, 20, 24, 28, or 32 bytes for valid BIP-39 word counts.
	validLengths := map[int]bool{16: true, 20: true, 24: true, 28: true, 32: true}
	if !validLengths[len(entropy)] {
		t.Fatalf("unexpected entropy length %d", len(entropy))
	}
}

func TestMnemonicToEntropy_InvalidMnemonicReturnsError(t *testing.T) {
	_, err := MnemonicToEntropy("not a valid mnemonic string at all")
	if err == nil {
		t.Fatal("expected error for invalid mnemonic, got nil")
	}
}

func TestMnemonicToEntropy_DifferentMnemonicsProduceDifferentEntropy(t *testing.T) {
	m1 := mustMnemonic(t)
	m2 := mustMnemonic(t)
	// Statistically impossible to collide, but guard anyway.
	if m1 == m2 {
		t.Skip("mnemonic collision – skipping determinism check")
	}
	e1, err := MnemonicToEntropy(m1)
	if err != nil {
		t.Fatalf("first MnemonicToEntropy: %v", err)
	}
	e2, err := MnemonicToEntropy(m2)
	if err != nil {
		t.Fatalf("second MnemonicToEntropy: %v", err)
	}
	if string(e1) == string(e2) {
		t.Fatal("different mnemonics produced identical entropy")
	}
}

// ---------------------------------------------------------------------------
// Proofs
// ---------------------------------------------------------------------------

// validProof returns a minimal Proof struct sufficient for quantity-based
// functions (ProofsTotalAmount, ProofHasDleq, ProofIsActive).
func validProof(amountSats uint64, keysetId string) Proof {
	return Proof{
		Amount:   Amount{Value: amountSats},
		Secret:   "deadbeef",
		C:        "02" + strings.Repeat("ab", 32),
		KeysetId: keysetId,
		Witness:  nil,
		Dleq:     nil,
		P2pkE:    nil,
	}
}

func TestProofsTotalAmount_Empty(t *testing.T) {
	total, err := ProofsTotalAmount([]Proof{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total.Value != 0 {
		t.Fatalf("expected 0 for empty slice, got %d", total.Value)
	}
}

func TestProofsTotalAmount_Single(t *testing.T) {
	proofs := []Proof{validProof(64, "000f01")}
	total, err := ProofsTotalAmount(proofs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total.Value != 64 {
		t.Fatalf("expected 64, got %d", total.Value)
	}
}

func TestProofsTotalAmount_MultipleProofs(t *testing.T) {
	proofs := []Proof{
		validProof(1, "000f01"),
		validProof(2, "000f01"),
		validProof(4, "000f01"),
		validProof(8, "000f01"),
	}
	total, err := ProofsTotalAmount(proofs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total.Value != 15 {
		t.Fatalf("expected 15, got %d", total.Value)
	}
}

func TestProofsTotalAmount_LargeAmount(t *testing.T) {
	const big = uint64(1<<32 - 1)
	proofs := []Proof{validProof(big, "000f01"), validProof(big, "000f01")}
	total, err := ProofsTotalAmount(proofs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total.Value != big*2 {
		t.Fatalf("expected %d, got %d", big*2, total.Value)
	}
}

func TestProofHasDleq_False(t *testing.T) {
	p := validProof(1, "000f01")
	if ProofHasDleq(p) {
		t.Fatal("expected ProofHasDleq to be false for a proof without DLEQ")
	}
}

func TestProofHasDleq_True(t *testing.T) {
	p := validProof(1, "000f01")
	p.Dleq = &ProofDleq{
		E: strings.Repeat("01", 32),
		S: strings.Repeat("02", 32),
		R: strings.Repeat("03", 32),
	}
	if !ProofHasDleq(p) {
		t.Fatal("expected ProofHasDleq to be true for a proof with DLEQ")
	}
}

func TestProofIsActive_MatchingKeysetId(t *testing.T) {
	p := validProof(1, "000f01")
	if !ProofIsActive(p, []string{"000f01", "000f02"}) {
		t.Fatal("expected proof to be active when its keyset ID is in the active list")
	}
}

func TestProofIsActive_NonMatchingKeysetId(t *testing.T) {
	p := validProof(1, "000f99")
	if ProofIsActive(p, []string{"000f01", "000f02"}) {
		t.Fatal("expected proof to be inactive when its keyset ID is not in the active list")
	}
}

func TestProofIsActive_EmptyActiveList(t *testing.T) {
	p := validProof(1, "000f01")
	if ProofIsActive(p, []string{}) {
		t.Fatal("expected proof to be inactive against an empty active keyset list")
	}
}

// ---------------------------------------------------------------------------
// MintQuote helpers
// ---------------------------------------------------------------------------

func makeMintQuote(state QuoteState, expiryUnix uint64) MintQuote {
	return MintQuote{
		Id:              "test-quote-id",
		Amount:          nil,
		Unit:            CurrencyUnitSat{},
		Request:         "lnbc...",
		State:           state,
		Expiry:          expiryUnix,
		MintUrl:         MintUrl{Url: "https://mint.example.com"},
		AmountIssued:    Amount{Value: 0},
		AmountPaid:      Amount{Value: 0},
		PaymentMethod:   PaymentMethodBolt11{},
		SecretKey:       nil,
		UsedByOperation: nil,
		Version:         0,
	}
}

func TestMintQuoteIsExpired_NotExpired(t *testing.T) {
	future := uint64(time.Now().Add(24 * time.Hour).Unix())
	quote := makeMintQuote(QuoteStateUnpaid, future)
	now := uint64(time.Now().Unix())
	expired, err := MintQuoteIsExpired(quote, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expired {
		t.Fatal("quote that expires in the future should not be expired")
	}
}

func TestMintQuoteIsExpired_Expired(t *testing.T) {
	past := uint64(time.Now().Add(-24 * time.Hour).Unix())
	quote := makeMintQuote(QuoteStateUnpaid, past)
	now := uint64(time.Now().Unix())
	expired, err := MintQuoteIsExpired(quote, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !expired {
		t.Fatal("quote that expired in the past should be expired")
	}
}

func TestMintQuoteAmountMintable_ZeroAmountPaid(t *testing.T) {
	future := uint64(time.Now().Add(24 * time.Hour).Unix())
	quote := makeMintQuote(QuoteStateUnpaid, future)
	mintable, err := MintQuoteAmountMintable(quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mintable.Value != 0 {
		t.Fatalf("expected 0 mintable sats for an unpaid quote, got %d", mintable.Value)
	}
}

func TestMintQuoteTotalAmount_ZeroAmountPaid(t *testing.T) {
	future := uint64(time.Now().Add(24 * time.Hour).Unix())
	quote := makeMintQuote(QuoteStateUnpaid, future)
	total, err := MintQuoteTotalAmount(quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total.Value != 0 {
		t.Fatalf("expected 0 total, got %d", total.Value)
	}
}

// ---------------------------------------------------------------------------
// Transaction helpers
// ---------------------------------------------------------------------------

func makeTransaction(dir TransactionDirection, mintURL string, unit CurrencyUnit) Transaction {
	return Transaction{
		Id:             TransactionId{Hex: strings.Repeat("a", 64)},
		MintUrl:        MintUrl{Url: mintURL},
		Direction:      dir,
		Amount:         Amount{Value: 100},
		Fee:            Amount{Value: 1},
		Unit:           unit,
		Ys:             []PublicKey{},
		Timestamp:      uint64(time.Now().Unix()),
		Memo:           nil,
		Metadata:       map[string]string{},
		QuoteId:        nil,
		PaymentRequest: nil,
		PaymentProof:   nil,
		PaymentMethod:  nil,
		SagaId:         nil,
	}
}

func TestTransactionMatchesConditions_AllNil(t *testing.T) {
	tx := makeTransaction(TransactionDirectionIncoming, "https://mint.example.com", CurrencyUnitSat{})
	match, err := TransactionMatchesConditions(tx, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !match {
		t.Fatal("transaction with all-nil filters should match")
	}
}

func TestTransactionMatchesConditions_MintUrlMatch(t *testing.T) {
	url := "https://mint.example.com"
	tx := makeTransaction(TransactionDirectionIncoming, url, CurrencyUnitSat{})
	mintUrl := MintUrl{Url: url}
	match, err := TransactionMatchesConditions(tx, &mintUrl, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !match {
		t.Fatal("transaction should match on exact mint URL")
	}
}

func TestTransactionMatchesConditions_MintUrlMismatch(t *testing.T) {
	tx := makeTransaction(TransactionDirectionIncoming, "https://mint.example.com", CurrencyUnitSat{})
	other := MintUrl{Url: "https://other.example.com"}
	match, err := TransactionMatchesConditions(tx, &other, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match {
		t.Fatal("transaction should not match on a different mint URL")
	}
}

func TestTransactionMatchesConditions_DirectionMatch(t *testing.T) {
	tx := makeTransaction(TransactionDirectionOutgoing, "https://mint.example.com", CurrencyUnitSat{})
	dir := TransactionDirectionOutgoing
	match, err := TransactionMatchesConditions(tx, nil, &dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !match {
		t.Fatal("transaction should match on correct direction")
	}
}

func TestTransactionMatchesConditions_DirectionMismatch(t *testing.T) {
	tx := makeTransaction(TransactionDirectionIncoming, "https://mint.example.com", CurrencyUnitSat{})
	dir := TransactionDirectionOutgoing
	match, err := TransactionMatchesConditions(tx, nil, &dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match {
		t.Fatal("transaction should not match on wrong direction")
	}
}

func TestTransactionMatchesConditions_UnitMatch(t *testing.T) {
	tx := makeTransaction(TransactionDirectionIncoming, "https://mint.example.com", CurrencyUnitSat{})
	var unit CurrencyUnit = CurrencyUnitSat{}
	match, err := TransactionMatchesConditions(tx, nil, nil, &unit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !match {
		t.Fatal("transaction should match on correct currency unit")
	}
}

func TestTransactionMatchesConditions_UnitMismatch(t *testing.T) {
	tx := makeTransaction(TransactionDirectionIncoming, "https://mint.example.com", CurrencyUnitSat{})
	var unit CurrencyUnit = CurrencyUnitMsat{}
	match, err := TransactionMatchesConditions(tx, nil, nil, &unit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match {
		t.Fatal("transaction should not match on wrong currency unit")
	}
}

// ---------------------------------------------------------------------------
// Token encode/decode roundtrip
// ---------------------------------------------------------------------------

// A valid v3 cashu token (sat, 8333.space mint, 2-sat proof).
// From the Cashu NUT-00 test vectors.
const sampleTokenV3 = "cashuAeyJ0b2tlbiI6W3sibWludCI6Imh0dHBzOi8vODMzMy5zcGFjZTozMzM4IiwicHJvb2ZzIjpbeyJhbW91bnQiOjIsImlkIjoiMDA5YTFmMjkzMjUzZTQxZSIsInNlY3JldCI6IjQwNzkxNWJjMjEyYmU2MWE3N2UzZTZkMmFlYjRjNzI3OTgwYmRhNTFjZDMwNmEyNTlkNGRhNDNiYWIxZmIiLCJDIjoiMDJiYzkwNTc5YTViOGJmODZhYThmZmQ3NjRkMTJjMzJhYmVkYzBhMWYzNTlhMjYwZDM2MGFhZGE2OWRkMzdhIn1dfV19"

func TestTokenDecode_ValidToken(t *testing.T) {
	tok, err := TokenDecode(sampleTokenV3)
	if err != nil {
		t.Fatalf("TokenDecode: %v", err)
	}
	if tok == nil {
		t.Fatal("decoded token must not be nil")
	}
}

func TestTokenDecode_EncodeRoundTrip(t *testing.T) {
	tok, err := TokenDecode(sampleTokenV3)
	if err != nil {
		t.Fatalf("TokenDecode: %v", err)
	}
	encoded := tok.Encode()
	if encoded == "" {
		t.Fatal("Encode returned empty string")
	}
	// Decode again to confirm the re-encoded form is valid.
	tok2, err := TokenDecode(encoded)
	if err != nil {
		t.Fatalf("re-decode after Encode: %v", err)
	}
	if tok2 == nil {
		t.Fatal("re-decoded token must not be nil")
	}
}

func TestTokenFromString_ValidToken(t *testing.T) {
	tok, err := TokenFromString(sampleTokenV3)
	if err != nil {
		t.Fatalf("TokenFromString: %v", err)
	}
	if tok == nil {
		t.Fatal("token must not be nil")
	}
}

func TestTokenDecode_InvalidToken(t *testing.T) {
	_, err := TokenDecode("not-a-valid-cashu-token")
	if err == nil {
		t.Fatal("expected error for invalid token string, got nil")
	}
}

func TestTokenDecode_MintUrl(t *testing.T) {
	tok, err := TokenDecode(sampleTokenV3)
	if err != nil {
		t.Fatalf("TokenDecode: %v", err)
	}
	mintUrl, err := tok.MintUrl()
	if err != nil {
		t.Fatalf("MintUrl: %v", err)
	}
	if mintUrl.Url == "" {
		t.Fatal("MintUrl must not be empty")
	}
}

func TestTokenDecode_Value(t *testing.T) {
	tok, err := TokenDecode(sampleTokenV3)
	if err != nil {
		t.Fatalf("TokenDecode: %v", err)
	}
	val, err := tok.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if val.Value == 0 {
		t.Fatal("token value must be > 0")
	}
}

func TestTokenDecode_ProofsSimple(t *testing.T) {
	tok, err := TokenDecode(sampleTokenV3)
	if err != nil {
		t.Fatalf("TokenDecode: %v", err)
	}
	proofs, err := tok.ProofsSimple()
	if err != nil {
		t.Fatalf("ProofsSimple: %v", err)
	}
	if len(proofs) == 0 {
		t.Fatal("expected at least one proof in token")
	}
}

func TestTokenFromRawBytes_RoundTrip(t *testing.T) {
	tok, err := TokenDecode(sampleTokenV3)
	if err != nil {
		t.Fatalf("TokenDecode: %v", err)
	}
	raw, err := tok.ToRawBytes()
	if err != nil {
		t.Fatalf("ToRawBytes: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("raw bytes must not be empty")
	}
	tok2, err := TokenFromRawBytes(raw)
	if err != nil {
		t.Fatalf("TokenFromRawBytes: %v", err)
	}
	if tok2 == nil {
		t.Fatal("reconstructed token must not be nil")
	}
}

// ---------------------------------------------------------------------------
// JSON encode / decode roundtrips
// ---------------------------------------------------------------------------

func TestEncodeDecode_MintVersion(t *testing.T) {
	original := MintVersion{Name: "Nutshell", Version: "0.15.0"}
	encoded, err := EncodeMintVersion(original)
	if err != nil {
		t.Fatalf("EncodeMintVersion: %v", err)
	}
	decoded, err := DecodeMintVersion(encoded)
	if err != nil {
		t.Fatalf("DecodeMintVersion: %v", err)
	}
	if decoded.Name != original.Name {
		t.Fatalf("Name mismatch: got %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Version != original.Version {
		t.Fatalf("Version mismatch: got %q, want %q", decoded.Version, original.Version)
	}
}

func TestEncodeDecode_MintVersion_EmptyFields(t *testing.T) {
	original := MintVersion{Name: "", Version: ""}
	encoded, err := EncodeMintVersion(original)
	if err != nil {
		t.Fatalf("EncodeMintVersion: %v", err)
	}
	decoded, err := DecodeMintVersion(encoded)
	if err != nil {
		t.Fatalf("DecodeMintVersion: %v", err)
	}
	if decoded.Name != "" || decoded.Version != "" {
		t.Fatalf("expected empty fields, got Name=%q Version=%q", decoded.Name, decoded.Version)
	}
}

func TestDecodeMintVersion_InvalidJSON(t *testing.T) {
	_, err := DecodeMintVersion("{not valid json}")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestEncodeDecode_KeySetInfo(t *testing.T) {
	original := KeySetInfo{
		Id:          "000f01",
		Unit:        CurrencyUnitSat{},
		Active:      true,
		InputFeePpk: 0,
	}
	encoded, err := EncodeKeySetInfo(original)
	if err != nil {
		t.Fatalf("EncodeKeySetInfo: %v", err)
	}
	decoded, err := DecodeKeySetInfo(encoded)
	if err != nil {
		t.Fatalf("DecodeKeySetInfo: %v", err)
	}
	if decoded.Id != original.Id {
		t.Fatalf("Id mismatch: got %q, want %q", decoded.Id, original.Id)
	}
	if decoded.Active != original.Active {
		t.Fatalf("Active mismatch: got %v, want %v", decoded.Active, original.Active)
	}
}

func TestEncodeDecode_Transaction(t *testing.T) {
	original := makeTransaction(TransactionDirectionIncoming, "https://mint.example.com", CurrencyUnitSat{})
	encoded, err := EncodeTransaction(original)
	if err != nil {
		t.Fatalf("EncodeTransaction: %v", err)
	}
	decoded, err := DecodeTransaction(encoded)
	if err != nil {
		t.Fatalf("DecodeTransaction: %v", err)
	}
	if decoded.Id.Hex != original.Id.Hex {
		t.Fatalf("Id mismatch: got %q, want %q", decoded.Id.Hex, original.Id.Hex)
	}
	if decoded.Amount.Value != original.Amount.Value {
		t.Fatalf("Amount mismatch: got %d, want %d", decoded.Amount.Value, original.Amount.Value)
	}
	if decoded.Direction != original.Direction {
		t.Fatalf("Direction mismatch: got %v, want %v", decoded.Direction, original.Direction)
	}
}

func TestDecodeTransaction_InvalidJSON(t *testing.T) {
	_, err := DecodeTransaction("this is not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestEncodeDecode_MintQuote(t *testing.T) {
	future := uint64(time.Now().Add(time.Hour).Unix())
	original := makeMintQuote(QuoteStatePaid, future)
	original.AmountPaid = Amount{Value: 100}
	encoded, err := EncodeMintQuote(original)
	if err != nil {
		t.Fatalf("EncodeMintQuote: %v", err)
	}
	decoded, err := DecodeMintQuote(encoded)
	if err != nil {
		t.Fatalf("DecodeMintQuote: %v", err)
	}
	if decoded.Id != original.Id {
		t.Fatalf("Id mismatch: got %q, want %q", decoded.Id, original.Id)
	}
	if decoded.State != original.State {
		t.Fatalf("State mismatch: got %v, want %v", decoded.State, original.State)
	}
}

// ---------------------------------------------------------------------------
// PaymentRequest (NUT-18) decode
// ---------------------------------------------------------------------------

func TestDecodePaymentRequest_InvalidString(t *testing.T) {
	_, err := DecodePaymentRequest("definitely-not-a-payment-request")
	if err == nil {
		t.Fatal("expected error for invalid payment request, got nil")
	}
}

// ---------------------------------------------------------------------------
// WalletSqliteDatabase construction
// ---------------------------------------------------------------------------

func TestNewWalletSqliteDatabase_InMemory(t *testing.T) {
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase(':memory:'): %v", err)
	}
	if db == nil {
		t.Fatal("database handle must not be nil")
	}
}

func TestNewWalletSqliteDatabase_FilePath(t *testing.T) {
	dir := t.TempDir()
	db, err := NewWalletSqliteDatabase(dir + "/test.db")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase(file): %v", err)
	}
	if db == nil {
		t.Fatal("database handle must not be nil")
	}
}

// ---------------------------------------------------------------------------
// Wallet construction (offline – no mint contact)
// ---------------------------------------------------------------------------

func TestNewWallet_InMemoryDB(t *testing.T) {
	mnemonic := mustMnemonic(t)
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	wallet, err := NewWallet(
		"https://mint.example.com",
		CurrencyUnitSat{},
		mnemonic,
		db,
		WalletConfig{TargetProofCount: nil},
	)
	if err != nil {
		t.Fatalf("NewWallet: %v", err)
	}
	if wallet == nil {
		t.Fatal("wallet must not be nil")
	}
}

func TestNewWallet_InvalidMintUrl(t *testing.T) {
	mnemonic := mustMnemonic(t)
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	_, err = NewWallet(
		"not-a-url",
		CurrencyUnitSat{},
		mnemonic,
		db,
		WalletConfig{TargetProofCount: nil},
	)
	if err == nil {
		t.Fatal("expected error for invalid mint URL, got nil")
	}
}

func TestNewWallet_InvalidMnemonic(t *testing.T) {
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	_, err = NewWallet(
		"https://mint.example.com",
		CurrencyUnitSat{},
		"bad mnemonic that is clearly invalid",
		db,
		WalletConfig{TargetProofCount: nil},
	)
	if err == nil {
		t.Fatal("expected error for invalid mnemonic, got nil")
	}
}

// ---------------------------------------------------------------------------
// WalletRepository construction
// ---------------------------------------------------------------------------

func TestNewWalletRepository_InMemoryDB(t *testing.T) {
	mnemonic := mustMnemonic(t)
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	repo, err := NewWalletRepository(mnemonic, db)
	if err != nil {
		t.Fatalf("NewWalletRepository: %v", err)
	}
	if repo == nil {
		t.Fatal("repository must not be nil")
	}
}

func TestNewWalletRepository_InvalidMnemonic(t *testing.T) {
	db, err := NewWalletSqliteDatabase(":memory:")
	if err != nil {
		t.Fatalf("NewWalletSqliteDatabase: %v", err)
	}
	_, err = NewWalletRepository("invalid mnemonic words here", db)
	if err == nil {
		t.Fatal("expected error for invalid mnemonic, got nil")
	}
}

// ---------------------------------------------------------------------------
// Npubcash key derivation (offline)
// ---------------------------------------------------------------------------

func TestNpubcashDeriveSecretKeyFromSeed_ValidSeed(t *testing.T) {
	seed := make([]byte, 64)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	sk, err := NpubcashDeriveSecretKeyFromSeed(seed)
	if err != nil {
		t.Fatalf("NpubcashDeriveSecretKeyFromSeed: %v", err)
	}
	if sk == "" {
		t.Fatal("secret key must not be empty")
	}
	// Hex-encoded 32-byte secret key = 64 hex characters.
	if len(sk) != 64 {
		t.Fatalf("expected 64-char hex secret key, got %d chars: %q", len(sk), sk)
	}
}

func TestNpubcashDeriveSecretKeyFromSeed_TooShort(t *testing.T) {
	_, err := NpubcashDeriveSecretKeyFromSeed([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error for seed that is too short, got nil")
	}
}

func TestNpubcashGetPubkey_FromDerivedKey(t *testing.T) {
	seed := make([]byte, 64)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	sk, err := NpubcashDeriveSecretKeyFromSeed(seed)
	if err != nil {
		t.Fatalf("NpubcashDeriveSecretKeyFromSeed: %v", err)
	}
	pk, err := NpubcashGetPubkey(sk)
	if err != nil {
		t.Fatalf("NpubcashGetPubkey: %v", err)
	}
	if pk == "" {
		t.Fatal("public key must not be empty")
	}
	// Nostr pubkeys are X-only (32 bytes = 64 hex chars).
	if len(pk) != 64 {
		t.Fatalf("unexpected pubkey length %d: %q", len(pk), pk)
	}
}

func TestNpubcashGetPubkey_InvalidKey(t *testing.T) {
	_, err := NpubcashGetPubkey("not-a-secret-key")
	if err == nil {
		t.Fatal("expected error for invalid secret key, got nil")
	}
}

func TestNpubcashDeriveSecretKeyFromSeed_Deterministic(t *testing.T) {
	seed := make([]byte, 64)
	for i := range seed {
		seed[i] = byte(i)
	}
	sk1, err := NpubcashDeriveSecretKeyFromSeed(seed)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	sk2, err := NpubcashDeriveSecretKeyFromSeed(seed)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if sk1 != sk2 {
		t.Fatalf("key derivation is not deterministic: %q != %q", sk1, sk2)
	}
}

// ---------------------------------------------------------------------------
// Checksum file integrity (meta-test)
// ---------------------------------------------------------------------------

func TestChecksumFile_Exists(t *testing.T) {
	// Verify the checksums file is present alongside the native libraries.
	// When tests are run from the package directory the relative path is valid.
	const path = "native/checksums.sha256"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("native/checksums.sha256 not found: %v\nRun 'make update-checksums' to generate it.", err)
	}
}
