package eostest

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/eoscanada/eos-go"
	"github.com/eoscanada/eos-go/ecc"
	"github.com/eoscanada/eos-go/system"
	"github.com/eoscanada/eosc/cli"
)

const charset = "abcdefghijklmnopqrstuvwxyz" + "12345"
const creator = "eosio"

var seededRand *rand.Rand = rand.New(
	rand.NewSource(time.Now().UnixNano()))

func stringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

func randAccountName() string {
	return stringWithCharset(12, charset)
}

func toAccount(in, field string) eos.AccountName {
	acct, err := cli.ToAccountName(in)
	ErrorCheck(fmt.Sprintf("invalid account format for %q", field), err)

	return acct
}

// DefaultKey returns the default EOSIO key
func DefaultKey() string {
	return "5KQwrPbwdL6PhXujxW37FSSQZ1JiwsST4cqQzDeyXtP79zkvFD3"
}

// ErrorCheck - too generic - need to improve this
// TODO: fix
func ErrorCheck(prefix string, err error) {
	if err != nil {
		fmt.Printf("ERROR: %s: %s\n", prefix, err)
		os.Exit(1)
	}
}

// ExecTrx executes a list of actions
func ExecTrx(ctx context.Context, api *eos.API, actions []*eos.Action) (string, error) {
	txOpts := &eos.TxOptions{}
	if err := txOpts.FillFromChain(ctx, api); err != nil {
		log.Printf("Error filling tx opts: %s", err)
		return "error", err
	}

	tx := eos.NewTransaction(actions, txOpts)

	_, packedTx, err := api.SignTransaction(ctx, tx, txOpts.ChainID, eos.CompressionNone)
	if err != nil {
		log.Printf("Error signing transaction: %s", err)
		return "error", err
	}

	response, err := api.PushTransaction(ctx, packedTx)
	if err != nil {
		log.Printf("Error pushing transaction: %s", err)
		return "error", err
	}
	trxID := hex.EncodeToString(response.Processed.ID)
	return trxID, nil
}

// CreateAccountFromString creates an specific account from a string name
func CreateAccountFromString(ctx context.Context, api *eos.API, accountName string) (eos.AccountName, error) {
	keyBag := api.Signer
	key, _ := ecc.NewRandomPrivateKey()

	acct := toAccount(accountName, "account to create")

	err := keyBag.ImportPrivateKey(ctx, key.String())
	if err != nil {
		log.Panicf("import private key: %s", err)
	}

	actions := []*eos.Action{system.NewNewAccount(creator, acct, key.PublicKey())}
	_, err = ExecTrx(ctx, api, actions)
	if err != nil {
		log.Panicf("cannot create random accounts: %s", err)
		return acct, err
	}

	codePermissionActions := []*eos.Action{system.NewUpdateAuth(acct,
		"active",
		"owner",
		eos.Authority{
			Threshold: 1,
			Keys: []eos.KeyWeight{{
				PublicKey: key.PublicKey(),
				Weight:    1,
			}},
			Accounts: []eos.PermissionLevelWeight{{
				Permission: eos.PermissionLevel{
					Actor:      acct,
					Permission: "eosio.code",
				},
				Weight: 1,
			}},
			Waits: []eos.WaitWeight{},
		}, "owner")}

	_, err = ExecTrx(ctx, api, codePermissionActions)
	if err != nil {
		log.Panicf("cannot create random accounts: %s", err)
		return acct, err
	}
	return acct, nil
}

// CreateRandoms returns a list of accounts with eosio.code permission attached to active
func CreateRandoms(ctx context.Context, api *eos.API, length int) ([]eos.AccountName, error) {

	accounts := make([]eos.AccountName, length)
	i := 0
	for i < length {
		account, err := CreateAccountFromString(ctx, api, randAccountName())
		if err != nil {
			log.Panicf("cannot create account: %s", err)
			return nil, err
		}
		accounts[i] = account
		i++
	}
	return accounts, nil
}

// SetContract sets the wasm and abi files to the account
func SetContract(ctx context.Context, api *eos.API, accountName eos.AccountName, wasmFile, abiFile string) (string, error) {
	setCodeAction, err := system.NewSetCode(accountName, wasmFile)
	ErrorCheck("loading wasm file", err)

	setAbiAction, err := system.NewSetABI(accountName, abiFile)
	ErrorCheck("loading abi file", err)

	return ExecTrx(ctx, api, []*eos.Action{setCodeAction, setAbiAction})
}

type tokenCreate struct {
	Issuer    eos.AccountName
	MaxSupply eos.Asset
}

// DeployAndCreateToken deploys the standard token contract and creates the specified token max supply
func DeployAndCreateToken(ctx context.Context, t *testing.T, api *eos.API, tokenHome string,
	contract, issuer eos.AccountName, maxSupply eos.Asset) (string, error) {

	// TODO: how to save wasm and abi to distribute with package
	_, err := SetContract(ctx, api, contract, tokenHome+"/token/token.wasm", tokenHome+"/token/token.abi")
	if err != nil {
		log.Panicf("cannot set contract: %s", err)
	}

	actions := []*eos.Action{{
		Account: contract,
		Name:    eos.ActN("create"),
		Authorization: []eos.PermissionLevel{
			{Actor: contract, Permission: eos.PN("active")},
		},
		ActionData: eos.NewActionData(tokenCreate{
			Issuer:    issuer,
			MaxSupply: maxSupply,
		}),
	}}

	t.Log("Created Token : ", contract, " 		: ", maxSupply.String())
	return ExecTrx(ctx, api, actions)
}
