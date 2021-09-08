package model

import "crypto/ecdsa"
import config "github.com/TRON-US/go-btfs-config"

type Config struct {
	Version      string
	AutoTransfer []AutoTransferWallet `yaml:"AutoTransfer"`
}

type AutoTransferWallet struct {
	Name      string `yaml:"name"`
	KeyType   string `yaml:"keyType"`
	KeyValue  string `yaml:"keyValue"`
	Recipient string `yaml:"recipient"`
	Interval  int64  `yaml:"interval"`
	Address   string `yaml:"-"`
}

type Address struct {
	Mnemonic        string            `json:"mnemonic"`
	PrivateKeyInHex string            `json:"privateKeyInHex"`
	PrivateKeyEcdsa *ecdsa.PrivateKey `json:"privateKeyEcdsa"`
	Base58Address   string            `json:"base58Address"`
	LedgerAddress   []byte            `json:"ledgerAddress"`
	TronAddress     []byte            `json:"tronAddress"`
	Identity        config.Identity   `json:"identity"`
}

type Balance struct {
	TrxBalance    int64            `json:"trxBalance"`
	LedgerBalance int64            `json:"ledgerBalance"`
	FreeNetUsage  int64            `json:"freeNetUsage"`
	TokenBalances map[string]int64 `json:"tokenBalances"`
}
