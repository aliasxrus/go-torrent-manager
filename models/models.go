package model

import "crypto/ecdsa"
import config "github.com/TRON-US/go-btfs-config"

type Config struct {
	Version      string
	AutoTransfer []AutoTransferWallet `yaml:"AutoTransfer"`
}

type AutoTransferWallet struct {
	KeyType   string `yaml:"keyType"`
	KeyValue  string `yaml:"keyValue"`
	Recipient string `yaml:"recipient"`
	Interval  string `yaml:"interval"`
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
