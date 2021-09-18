package model

import "crypto/ecdsa"
import config "github.com/TRON-US/go-btfs-config"

type Config struct {
	Version             string
	AutoTransferWallets []AutoTransferWallet `yaml:"AutoTransferWallets"`
	AutoWithdrawWallets []AutoWithdrawWallet `yaml:"AutoWithdrawWallets"`
	AutoWithdrawConfig  AutoWithdrawConfig   `yaml:"AutoWithdrawConfig"`
}

type AutoTransferWallet struct {
	Name                string `yaml:"name"`
	KeyType             string `yaml:"keyType"`
	KeyValue            string `yaml:"keyValue"`
	Recipient           string `yaml:"recipient"`
	Interval            int64  `yaml:"interval"`
	TmmRecipientAddress string `yaml:"tmmRecipientAddress"` // TRON for TMM
	Sum                 int64  `yaml:"-"`
}

type AutoWithdrawWallet struct {
	Name                string  `yaml:"name"`
	KeyType             string  `yaml:"keyType"`
	KeyValue            string  `yaml:"keyValue"`
	Strategy            int64   `yaml:"strategy"` // balance, in, out, diff
	Difference          int64   `yaml:"difference"`
	MinAmount           int64   `yaml:"minAmount"`
	Second              []int   `yaml:"second"`
	BttRecipientAddress string  `yaml:"bttRecipientAddress"` // Адрес получателя BTT
	Address             Address `yaml:"-"`
	GatewayBalance      Balance `yaml:"-"`
	LedgerBalance       int64   `yaml:"-"`
}

type AutoWithdrawConfig struct {
	Interval int64  `yaml:"interval"`
	Url      string `yaml:"url"`
	Refresh  int64  `yaml:"refresh"` // Через какое количество интервалов обновлять балансы кошельков
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
	BttBalance    int64            `json:"bttBalance"`
	LedgerBalance int64            `json:"ledgerBalance"`
	FreeNetUsage  int64            `json:"freeNetUsage"`
	TokenBalances map[string]int64 `json:"tokenBalances"`
}

type TronScanResponse struct {
	TokenBalances []struct {
		TokenId string `json:"tokenId"`
		Balance string `json:"balance"`
	} `json:"tokenBalances"`
	Bandwidth struct {
		FreeNetRemaining int64 `json:"freeNetRemaining"`
		FreeNetUsed      int64 `json:"freeNetUsed"`
	} `json:"bandwidth"`
	Data []struct {
		AssetV2 []struct {
			Key   string `json:"key"`
			Value int64  `json:"value"`
		} `json:"assetV2"`
		Balance      int64 `json:"balance"`
		FreeNetUsage int64 `json:"free_net_usage"`
	} `json:"data"`
}
