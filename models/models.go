package model

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
