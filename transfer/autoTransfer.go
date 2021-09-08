package transfer

import (
	"github.com/beego/beego/v2/core/logs"
	"go-torrent-manager/btfs/util"
	"go-torrent-manager/conf"
	model "go-torrent-manager/models"
	"os"
)

func init() {
	config := conf.Get()

	for _, transferWallet := range config.AutoTransfer {
		go transfer(transferWallet)
	}
}

func transfer(transferWallet model.AutoTransferWallet) {
	address, err := util.GetAddress(transferWallet.KeyType, transferWallet.KeyValue)
	if err != nil {
		logs.Error("Generate key for transfer.", err)
		os.Exit(1)
	}
	logs.Info("Transfer added:", address.Base58Address, ", Interval:", transferWallet.Interval, ", Recipient:", transferWallet.Recipient)
}
